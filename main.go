package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LogicSamurai/Chirpy/internal/auth"
	"github.com/LogicSamurai/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	DB             *database.Queries
	Platform       string
	Jwt_secret	string
	Polka_key	string
}

func (cfg *apiConfig) polkaWebhookHandler(response http.ResponseWriter, request *http.Request) {

	apiKey, err := auth.GetAPIKey(request.Header)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	if apiKey != cfg.Polka_key {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}
	
	type requestFormat struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(request.Body)
	requestData := requestFormat{}

	if err := decoder.Decode(&requestData); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	if requestData.Event != "user.upgraded" {
		response.WriteHeader(http.StatusNoContent)
		return
	}

	_, err = cfg.DB.UpdateUserToChirpyRed(request.Context(),uuid.MustParse(requestData.Data.UserID))
	if err != nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	response.WriteHeader(http.StatusNoContent)

}

func (cfg *apiConfig) deleteChirpHandler(response http.ResponseWriter, request *http.Request) {
	token, err := auth.GetBearerToken(request.Header)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.Jwt_secret)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	chirpId := uuid.MustParse(request.PathValue("chirpID"))
	chirp, err := cfg.DB.GetChirpById(request.Context(), chirpId)
	if err != nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	// Only the author can delete their own chirp
	if chirp.UserID != userId {
		response.WriteHeader(http.StatusForbidden)
		return
	}

	err = cfg.DB.DeleteChirpById(request.Context(), chirpId)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) updateUserHandler(response http.ResponseWriter, request *http.Request) {
	token, err := auth.GetBearerToken(request.Header)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.Jwt_secret)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	type requestBody struct {
		Email string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(request.Body)
	requestBodyData := requestBody{}

	if err := decoder.Decode(&requestBodyData); err != nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}
	
	hashedPass, err := auth.HashPassword(requestBodyData.Password)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
	
	updateUserDetailsParams := database.UpdateUserDetailsParams {
		Email: requestBodyData.Email,
		HashedPassword: hashedPass,
		ID: userId,
	}

	updatedUserDetails, err := cfg.DB.UpdateUserDetails(request.Context(), updateUserDetailsParams)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	type responseDataParams struct {
		ID uuid.UUID `json:"id"`
		Email string `json:"email"`
	}

	responseData := responseDataParams {
		ID: updatedUserDetails.ID,
		Email: updateUserDetailsParams.Email,
	}

	data, err := json.Marshal(responseData)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	response.Write(data)
}

func (cfg *apiConfig) revokeHandler(response http.ResponseWriter, request *http.Request) {
	token, err := auth.GetBearerToken(request.Header)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	UpdateRefreshTokenParams := database.UpdateRefreshTokenParams{
		RevokedAt: sql.NullTime{Time: time.Now(), Valid: true},
		Token:     token,
	}
	err = cfg.DB.UpdateRefreshToken(request.Context(), UpdateRefreshTokenParams)

	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) refreshHandler(response http.ResponseWriter, request *http.Request) {
	token, err := auth.GetBearerToken(request.Header)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	refreshTokenDetails, err := cfg.DB.GetRefreshTokenDetails(request.Context(), token)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	if time.Until(refreshTokenDetails.ExpiresAt) <= 0 || refreshTokenDetails.RevokedAt.Valid {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	newAccessToken, err := auth.MakeJWT(refreshTokenDetails.UserID, cfg.Jwt_secret, time.Duration(3600) * time.Second)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	type responseBody struct {
		Token string `json:"token"`
	}

	resBody := responseBody{
		Token: newAccessToken,
	}

	data, err := json.Marshal(resBody)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	response.Write(data)
}

func (cfg *apiConfig) loginHandler(response http.ResponseWriter, request *http.Request) {
	type requestBody struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(request.Body)
	requestData := requestBody{}


	if err := decoder.Decode(&requestData); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
	}

	fmt.Printf("EMAIL %v pASSWOR: %v ================\n", requestData.Email, requestData.Password)

	user, err := cfg.DB.GetUserByEmail(request.Context(), requestData.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
				// User doesn't exist
				response.WriteHeader(http.StatusUnauthorized) // or StatusNotFound depending on your API
				return
		}
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	checkPassword, err := auth.CheckPasswordHash(requestData.Password, user.HashedPassword)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !checkPassword {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	type responseFormat struct {
		Id           string    `json:"id"`
		Created_at   time.Time `json:"created_at"`
		Updated_at   time.Time `json:"updated_at"`
		Email      	 string    `json:"email"`
		IsChirpyRed  bool 	   `json:"is_chirpy_red"`
		Token	     string	   `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}

	access_token_expire_duration, refresh_token_expire_duration := 3600, 5184000	
	access_token, err := auth.MakeJWT(user.ID, cfg.Jwt_secret, time.Duration(access_token_expire_duration) * time.Second)	
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return		
	}

	refresh_token := auth.MakeRefreshToken()

	refreshTokenParams := database.CreatRefreshTokenParams {
		Token : refresh_token,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UserID: user.ID,
		ExpiresAt: time.Now().Add(time.Duration(refresh_token_expire_duration) * time.Second),
		RevokedAt: sql.NullTime{},
	}

	err = cfg.DB.CreatRefreshToken(request.Context(),refreshTokenParams)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
	
	responseBody := responseFormat{
		Id:         user.ID.String(),
		Created_at: user.CreatedAt,
		Updated_at: user.UpdatedAt,
		Email:      user.Email,
		IsChirpyRed: user.IsChirpyRed.Bool,
		Token: access_token,
		RefreshToken: refresh_token,
	}

	data, err := json.Marshal(responseBody)
	if err != nil {
		fmt.Printf("Error marshalling JSON: %s", err)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
	response.WriteHeader(http.StatusOK)
	response.Write(data)
}

func (cfg *apiConfig) getChirpByIdHandler(response http.ResponseWriter, request *http.Request) {
	chirp, err := cfg.DB.GetChirpById(request.Context(), uuid.MustParse(request.PathValue("id")))
	if err != nil {
		fmt.Printf("Error getting chirp by id: %v\n", err)
		response.WriteHeader(http.StatusNotFound)
		return
	}

	type responseFormat struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	resBody := responseFormat{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}

	data, err := json.Marshal(resBody)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	response.Write(data)
}

func (cfg *apiConfig) getChirpsHandler(response http.ResponseWriter, request *http.Request) {

	chirps, err := cfg.DB.GetChirps(request.Context())
	if err != nil {
		fmt.Printf("Error getting chirps: %v\n", err)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	type responseFormat struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	var resBody []responseFormat

	// First sort the chirps slices as per created_at so we can have correct order
	slices.SortFunc(chirps, func(a, b database.Chirp) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	})

	for _, chirp := range chirps {
		resBody = append(resBody, responseFormat{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		})
	}

	// responseBody := responseFormat{
	// 	ID:        chirp.ID,
	// 	CreatedAt: chirp.CreatedAt,
	// 	UpdatedAt: chirp.UpdatedAt,
	// 	Body:      chirp.Body,
	// 	UserID:    chirp.UserID,
	// }

	data, err := json.Marshal(resBody)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	response.Write(data)
}

func (cfg *apiConfig) createChirpHandler(response http.ResponseWriter, request *http.Request) {
	// Debug: Check what header we're getting
    authHeader := request.Header.Get("Authorization")
    fmt.Printf("Raw Auth Header: '%s'\n", authHeader)
    
    token, err := auth.GetBearerToken(request.Header)
    if err != nil {
        fmt.Printf("GetBearerToken Error: %v\n", err)
        response.WriteHeader(http.StatusUnauthorized)
        return
    }
    
    fmt.Printf("Extracted Token: '%s'\n", token)
    
    userID, err := auth.ValidateJWT(token, cfg.Jwt_secret)
    if err != nil {
        fmt.Printf("ValidateJWT Error: %v\n", err)
        response.WriteHeader(http.StatusUnauthorized)
        return
    }
    
    fmt.Printf("Validated userID: %s\n", userID)

	type requestBody struct {
		Body   string    `json:"body"`
	}

	decoder := json.NewDecoder(request.Body)
	requestData := requestBody{}

	if err := decoder.Decode(&requestData); err != nil {
		type errorResponse struct {
			Error string `json:"error"`
		}

		data, _ := json.Marshal(errorResponse{
			Error: "Something went wrong",
		})

		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusBadRequest)
		response.Write(data)
		return
	}

	// Validate chirp length
	if len(requestData.Body) > 140 {
		type errorResponse struct {
			Error string `json:"error"`
		}

		data, _ := json.Marshal(errorResponse{
			Error: "Chirp is too long",
		})

		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusBadRequest)
		response.Write(data)
		return
	}

	// Clean profanity
	cleanedBody := strings.Clone(requestData.Body)

	badWords := map[string]bool{
		"kerfuffle": true,
		"sharbert":  true,
		"fornax":    true,
	}

	words := strings.Split(cleanedBody, " ")
	for i, word := range words {
		if badWords[strings.ToLower(word)] {
			words[i] = "****"
		}
	}
	cleanedBody = strings.Join(words, " ")

	// Check if user exists
	_, err = cfg.DB.GetUserById(request.Context(), userID)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	id, err := uuid.NewUUID()
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	chirpParams := database.CreateChirpParams{
		ID:        id,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Body:      cleanedBody,
		UserID:    userID,
	}

	chirp, err := cfg.DB.CreateChirp(request.Context(), chirpParams)
	if err != nil {
		fmt.Printf("Error creating chirp: %v\n", err)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	type responseFormat struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	responseBody := responseFormat{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}

	data, err := json.Marshal(responseBody)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(201)
	response.Write(data)
}

func (cfg *apiConfig) createUserHandler(response http.ResponseWriter, request *http.Request) {
	type requestBody struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(request.Body)
	requestData := requestBody{}

	err := decoder.Decode(&requestData)
	if err != nil {
		response.WriteHeader(501)
		response.Write([]byte("An error occured while decoding"))
		return
	}

	uuid, err := uuid.NewUUID()
	if err != nil {
		fmt.Printf("Error Occured %v\n", err)
		return
	}

	hashedPassword, err := auth.HashPassword(requestData.Password)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	userParams := database.CreateUserParams{
		ID:             uuid,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Email:          requestData.Email,
		HashedPassword: hashedPassword,
	}

	user, err := cfg.DB.CreateUser(request.Context(), userParams)
	if err != nil {
		fmt.Printf("An error occured while creating user: %v\n", err)
		return
	}

	response.WriteHeader(201)

	type responseFormat struct {
		Id         	string    `json:"id"`
		Created_at 	time.Time `json:"created_at"`
		Updated_at 	time.Time `json:"updated_at"`
		Email      	string    `json:"email"`
		IsChirpyRed bool	  `json:"is_chirpy_red"`
	}

	responseBody := responseFormat{
		Id:         	user.ID.String(),
		Created_at: 	user.CreatedAt,
		Updated_at: 	user.UpdatedAt,
		Email:      	user.Email,
		IsChirpyRed: 	user.IsChirpyRed.Bool,
	}

	data, err := json.Marshal(responseBody)
	if err != nil {
		fmt.Printf("Error marshalling JSON: %s", err)
		response.WriteHeader(500)
		return
	}
	response.WriteHeader(201)
	response.Write(data)
}

func (cfg *apiConfig) resetCountHandler(response http.ResponseWriter, request *http.Request) {
	if cfg.Platform != "dev" {
		response.WriteHeader(403)
		return
	}

	cfg.fileserverHits.Store(0)
	_, err := cfg.DB.DeleteUsers(request.Context())
	if err != nil {
		fmt.Printf("An error occured while deleting users: %v\n", err)
		response.WriteHeader(500)
		return
	}

	response.WriteHeader(200)
}

func (cfg *apiConfig) requestCountHandler(response http.ResponseWriter, request *http.Request) {
	// resString := fmt.Sprintf("Hits: %v\n",cfg.fileserverHits.Load())
	resString := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())
	response.Header().Set("Content-Type", "text/html")
	response.Write([]byte(resString))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func healthzHandler(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	response.WriteHeader(200)
	response.Write([]byte("OK"))

}

func main() {
	godotenv.Load()
	
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	jwt_secret := os.Getenv("JWT_SECRET")
	polka_key := os.Getenv("POLKA_KEY")
	
	db, err := sql.Open("postgres", dbURL)
	dbQueries := database.New(db)

	mux := http.NewServeMux()
	cfg := apiConfig{
		DB:       dbQueries,
		Platform: platform,
		Jwt_secret: jwt_secret,
		Polka_key: polka_key,
	}

	mux.Handle("/app/", http.StripPrefix("/app/", cfg.middlewareMetricsInc(http.FileServer(http.Dir("app")))))
	mux.HandleFunc("GET /api/healthz", healthzHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.requestCountHandler)
	mux.HandleFunc("POST /admin/reset", cfg.resetCountHandler)
	// mux.HandleFunc("POST /api/validate_chirp", validateChirpHandler)
	mux.HandleFunc("POST /api/users", cfg.createUserHandler)
	mux.HandleFunc("POST /api/chirps", cfg.createChirpHandler)
	mux.HandleFunc("GET /api/chirps", cfg.getChirpsHandler)
	mux.HandleFunc("GET /api/chirps/{id}", cfg.getChirpByIdHandler)
	mux.HandleFunc("POST /api/login", cfg.loginHandler)
	mux.HandleFunc("POST /api/refresh", cfg.refreshHandler)
	mux.HandleFunc("POST /api/revoke", cfg.revokeHandler)
	mux.HandleFunc("PUT /api/users", cfg.updateUserHandler)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}",cfg.deleteChirpHandler)
	mux.HandleFunc("POST /api/polka/webhooks", cfg.polkaWebhookHandler)

	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	fmt.Println("Hello how are you?")
	err = server.ListenAndServe()
	if err != nil {
		fmt.Printf("%v\n", err)
	}
}
