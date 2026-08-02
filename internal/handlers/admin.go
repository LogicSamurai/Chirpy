package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/LogicSamurai/Chirpy/internal/auth"
	"github.com/google/uuid"
)

// ResetHandler resets the hit counter and (dev only) wipes the database.
func (cfg *APIConfig) ResetHandler(response http.ResponseWriter, request *http.Request) {
	if cfg.Platform != "dev" {
		response.WriteHeader(http.StatusForbidden)
		return
	}

	cfg.fileserverHits.Store(0)
	if _, err := cfg.DB.DeleteUsers(request.Context()); err != nil {
		log.Printf("Error deleting users: %v", err)
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	response.WriteHeader(http.StatusOK)
}

// MetricsHandler renders a tiny HTML admin page showing the hit count.
func (cfg *APIConfig) MetricsHandler(response http.ResponseWriter, request *http.Request) {
	resString := fmt.Sprintf(
		"<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>",
		cfg.fileserverHits.Load(),
	)
	response.Header().Set("Content-Type", "text/html")
	response.Write([]byte(resString))
}

// PolkaWebhookHandler processes Polka webhook events (e.g. user.upgraded).
func (cfg *APIConfig) PolkaWebhookHandler(response http.ResponseWriter, request *http.Request) {
	apiKey, err := auth.GetAPIKey(request.Header)
	if err != nil || apiKey != cfg.polkaKey {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}

	type requestData struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}

	params := requestData{}
	if err := json.NewDecoder(request.Body).Decode(&params); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}

	if params.Event != "user.upgraded" {
		response.WriteHeader(http.StatusNoContent)
		return
	}

	if _, err = cfg.DB.UpdateUserToChirpyRed(request.Context(), uuid.MustParse(params.Data.UserID)); err != nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}
