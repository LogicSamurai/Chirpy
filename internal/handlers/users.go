package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/LogicSamurai/Chirpy/internal/auth"
	"github.com/LogicSamurai/Chirpy/internal/database"
	"github.com/google/uuid"
)

// userResponse is the public JSON shape for a user (no hashed password).
type userResponse struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
}

// CreateUserHandler registers a new user with a hashed password.
func (cfg *APIConfig) CreateUserHandler(response http.ResponseWriter, request *http.Request) {
	type requestBody struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	requestData := requestBody{}
	if err := json.NewDecoder(request.Body).Decode(&requestData); err != nil {
		respondWithError(response, http.StatusBadRequest, "Something went wrong")
		return
	}

	hashedPassword, err := auth.HashPassword(requestData.Password)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, err := cfg.DB.CreateUser(request.Context(), database.CreateUserParams{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Email:          requestData.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		log.Printf("Error creating user: %v", err)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	respondWithJSON(response, http.StatusCreated, userResponse{
		ID:          user.ID.String(),
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed.Bool,
	})
}

// LoginHandler authenticates a user and returns access + refresh tokens.
func (cfg *APIConfig) LoginHandler(response http.ResponseWriter, request *http.Request) {
	type requestBody struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	requestData := requestBody{}
	if err := json.NewDecoder(request.Body).Decode(&requestData); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, err := cfg.DB.GetUserByEmail(request.Context(), requestData.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.WriteHeader(http.StatusUnauthorized)
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

	const accessTokenExpiry = time.Hour
	const refreshTokenExpiry = 60 * 24 * time.Hour

	accessToken, err := auth.MakeJWT(user.ID, cfg.jwtSecret, accessTokenExpiry)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	refreshToken := auth.MakeRefreshToken()

	if err = cfg.DB.CreatRefreshToken(request.Context(), database.CreatRefreshTokenParams{
		Token:     refreshToken,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(refreshTokenExpiry),
		RevokedAt: sql.NullTime{},
	}); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	respondWithJSON(response, http.StatusOK, struct {
		userResponse
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}{
		userResponse: userResponse{
			ID:          user.ID.String(),
			CreatedAt:   user.CreatedAt,
			UpdatedAt:   user.UpdatedAt,
			Email:       user.Email,
			IsChirpyRed: user.IsChirpyRed.Bool,
		},
		Token:        accessToken,
		RefreshToken: refreshToken,
	})
}

// UpdateUserHandler updates the email and password of the authenticated user.
func (cfg *APIConfig) UpdateUserHandler(response http.ResponseWriter, request *http.Request) {
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
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	requestData := requestBody{}
	if err := json.NewDecoder(request.Body).Decode(&requestData); err != nil {
		respondWithError(response, http.StatusBadRequest, "Something went wrong")
		return
	}

	hashedPassword, err := auth.HashPassword(requestData.Password)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	updatedUser, err := cfg.DB.UpdateUserDetails(request.Context(), database.UpdateUserDetailsParams{
		Email:          requestData.Email,
		HashedPassword: hashedPassword,
		ID:             userID,
	})
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	respondWithJSON(response, http.StatusOK, struct {
		ID    uuid.UUID `json:"id"`
		Email string    `json:"email"`
	}{
		ID:    updatedUser.ID,
		Email: updatedUser.Email,
	})
}
