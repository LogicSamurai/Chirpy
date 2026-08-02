package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/LogicSamurai/Chirpy/internal/database"
	"github.com/LogicSamurai/Chirpy/internal/handlers"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	godotenv.Load()

	db, err := sql.Open("postgres", os.Getenv("DB_URL"))
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	cfg := handlers.New(
		database.New(db),
		os.Getenv("PLATFORM"),
		os.Getenv("JWT_SECRET"),
		os.Getenv("POLKA_KEY"),
	)

	mux := http.NewServeMux()
	mux.Handle("/app/", http.StripPrefix("/app/", cfg.MiddlewareMetricsInc(http.FileServer(http.Dir("app")))))
	mux.HandleFunc("GET /api/healthz", handlers.HealthzHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.MetricsHandler)
	mux.HandleFunc("POST /admin/reset", cfg.ResetHandler)

	mux.HandleFunc("POST /api/users", cfg.CreateUserHandler)
	mux.HandleFunc("PUT /api/users", cfg.UpdateUserHandler)
	mux.HandleFunc("POST /api/login", cfg.LoginHandler)

	mux.HandleFunc("POST /api/chirps", cfg.CreateChirpHandler)
	mux.HandleFunc("GET /api/chirps", cfg.GetChirpsHandler)
	mux.HandleFunc("GET /api/chirps/{id}", cfg.GetChirpByIDHandler)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", cfg.DeleteChirpByIDHandler)

	mux.HandleFunc("POST /api/refresh", cfg.RefreshHandler)
	mux.HandleFunc("POST /api/revoke", cfg.RevokeHandler)

	mux.HandleFunc("POST /api/polka/webhooks", cfg.PolkaWebhookHandler)

	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}
	log.Printf("Chirpy server listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
