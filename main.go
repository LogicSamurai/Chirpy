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
		Id         string    `json:"id"`
		Created_at time.Time `json:"created_at"`
		Updated_at time.Time `json:"updated_at"`
		Email      string    `json:"email"`
	}

	responseBody := responseFormat{
		Id:         user.ID.String(),
		Created_at: user.CreatedAt,
		Updated_at: user.UpdatedAt,
		Email:      user.Email,
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
		fmt.Printf("Error creating chirp: %v\n", err)
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
	type requestBody struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
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
	_, err := cfg.DB.GetUserById(request.Context(), requestData.UserID)
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
		UserID:    requestData.UserID,
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
		Id         string    `json:"id"`
		Created_at time.Time `json:"created_at"`
		Updated_at time.Time `json:"updated_at"`
		Email      string    `json:"email"`
	}

	responseBody := responseFormat{
		Id:         user.ID.String(),
		Created_at: user.CreatedAt,
		Updated_at: user.UpdatedAt,
		Email:      user.Email,
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
		fmt.Errorf("An error occured while deleting users: %v\n", err)
		response.WriteHeader(501)
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
	db, err := sql.Open("postgres", dbURL)
	dbQueries := database.New(db)

	mux := http.NewServeMux()
	cfg := apiConfig{
		DB:       dbQueries,
		Platform: platform,
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
