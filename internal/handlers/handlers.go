package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/LogicSamurai/Chirpy/internal/database"
)

// APIConfig holds shared dependencies and configuration that every handler
// needs access to (the database, the JWT secret, the Polka API key, etc.).
type APIConfig struct {
	fileserverHits atomic.Int32
	DB             *database.Queries
	Platform       string
	jwtSecret      string
	polkaKey       string
}

// New constructs an APIConfig from the values loaded from the environment in main.
func New(db *database.Queries, platform, jwtSecret, polkaKey string) *APIConfig {
	return &APIConfig{
		DB:        db,
		Platform:  platform,
		jwtSecret: jwtSecret,
		polkaKey:  polkaKey,
	}
}

// respondWithJSON marshals payload as JSON and writes it with the given status code.
func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

// respondWithError writes a JSON error body like {"error": msg} with the given status code.
func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w, code, map[string]string{"error": msg})
}
