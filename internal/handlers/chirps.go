package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"fmt"
	"strings"
	"time"

	"github.com/LogicSamurai/Chirpy/internal/auth"
	"github.com/LogicSamurai/Chirpy/internal/database"
	"github.com/google/uuid"
)

// chirpResponse is the JSON shape returned for a single chirp.
type chirpResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

// CreateChirpHandler authenticates the user, validates/cleans the body, and creates a chirp.
func (cfg *APIConfig) CreateChirpHandler(response http.ResponseWriter, request *http.Request) {
	token, err := auth.GetBearerToken(request.Header)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	type requestBody struct {
		Body string `json:"body"`
	}

	requestData := requestBody{}
	if err := json.NewDecoder(request.Body).Decode(&requestData); err != nil {
		respondWithError(response, http.StatusBadRequest, "Something went wrong")
		return
	}

	// Validate chirp length
	if len(requestData.Body) > 140 {
		respondWithError(response, http.StatusBadRequest, "Chirp is too long")
		return
	}

	cleanedBody := cleanProfanity(requestData.Body)

	// Ensure the authenticated user still exists
	if _, err = cfg.DB.GetUserById(request.Context(), userID); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	chirp, err := cfg.DB.CreateChirp(request.Context(), database.CreateChirpParams{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Body:      cleanedBody,
		UserID:    userID,
	})
	if err != nil {
		log.Printf("Error creating chirp: %v", err)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	respondWithJSON(response, http.StatusCreated, chirpResponse{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	})
}

// GetChirpsHandler returns all chirps sorted by created_at ascending.
func (cfg *APIConfig) GetChirpsHandler(response http.ResponseWriter, request *http.Request) {
	queryParams := request.URL.Query()
	author_id := queryParams.Get("author_id")
	fmt.Println(author_id, "=====DSFDSFDSHG DSGH DSJ ======")

	if author_id != "" {
		sort := queryParams.Get("sort")
		if sort == "" {
			sort = "asc"
		}
		
		authorChirps, err := cfg.DB.GetChirpsForUser(request.Context(), database.GetChirpsForUserParams{
			UserID: uuid.MustParse(author_id),
			SortDirection: sort,
		})
	 	if err != nil {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		resp := make([]chirpResponse, 0, len(authorChirps))
		for _,chirp := range authorChirps {
			resp = append(resp, chirpResponse{
				ID: chirp.ID,
				CreatedAt: chirp.CreatedAt,
				UpdatedAt: chirp.UpdatedAt,
				Body: chirp.Body,
				UserID: chirp.UserID,
			})
		}

		respondWithJSON(response, http.StatusOK, resp)
		return
	}

	sort := queryParams.Get("sort")
	if sort == "" {
		sort = "asc"
	}
	
	chirps, err := cfg.DB.GetChirps(request.Context(),sort)
	if err != nil {
		log.Printf("Error getting chirps: %v", err)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	// slices.SortFunc(chirps, func(a, b database.Chirp) int {
	// 	return a.CreatedAt.Compare(b.CreatedAt)
	// })
	// 

	resp := make([]chirpResponse, 0, len(chirps))
	for _, chirp := range chirps {
		resp = append(resp, chirpResponse{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		})
	}

	respondWithJSON(response, http.StatusOK, resp)
}

// GetChirpByIDHandler returns a single chirp by its id path parameter.
func (cfg *APIConfig) GetChirpByIDHandler(response http.ResponseWriter, request *http.Request) {
	chirp, err := cfg.DB.GetChirpById(request.Context(), uuid.MustParse(request.PathValue("id")))
	if err != nil {
		log.Printf("Error getting chirp by id: %v", err)
		response.WriteHeader(http.StatusNotFound)
		return
	}

	respondWithJSON(response, http.StatusOK, chirpResponse{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	})
}

// DeleteChirpByIDHandler deletes a chirp, but only if it belongs to the authenticated user.
func (cfg *APIConfig) DeleteChirpByIDHandler(response http.ResponseWriter, request *http.Request) {
	token, err := auth.GetBearerToken(request.Header)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	chirpID := uuid.MustParse(request.PathValue("chirpID"))
	chirp, err := cfg.DB.GetChirpById(request.Context(), chirpID)
	if err != nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	// Only the author can delete their own chirp
	if chirp.UserID != userID {
		response.WriteHeader(http.StatusForbidden)
		return
	}

	if err = cfg.DB.DeleteChirpById(request.Context(), chirpID); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}

// cleanProfanity replaces known bad words with ****.
func cleanProfanity(body string) string {
	badWords := map[string]bool{
		"kerfuffle": true,
		"sharbert":  true,
		"fornax":    true,
	}

	words := strings.Split(body, " ")
	for i, word := range words {
		if badWords[strings.ToLower(word)] {
			words[i] = "****"
		}
	}
	return strings.Join(words, " ")
}
