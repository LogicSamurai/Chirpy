package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/LogicSamurai/Chirpy/internal/auth"
	"github.com/LogicSamurai/Chirpy/internal/database"
)

// RefreshHandler exchanges a valid refresh token for a new access token.
func (cfg *APIConfig) RefreshHandler(response http.ResponseWriter, request *http.Request) {
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

	newAccessToken, err := auth.MakeJWT(refreshTokenDetails.UserID, cfg.jwtSecret, time.Hour)
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	respondWithJSON(response, http.StatusOK, struct {
		Token string `json:"token"`
	}{
		Token: newAccessToken,
	})
}

// RevokeHandler revokes the supplied refresh token.
func (cfg *APIConfig) RevokeHandler(response http.ResponseWriter, request *http.Request) {
	token, err := auth.GetBearerToken(request.Header)
	if err != nil {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	err = cfg.DB.UpdateRefreshToken(request.Context(), database.UpdateRefreshTokenParams{
		RevokedAt: sql.NullTime{Time: time.Now(), Valid: true},
		Token:     token,
	})
	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}
