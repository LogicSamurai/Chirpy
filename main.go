package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}


func validateChirpHandler(response http.ResponseWriter, request *http.Request) {
	type requestBody struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(request.Body)
	requestData := requestBody{}

	err := decoder.Decode(&requestData)
	if err != nil {
		type returnVals struct {
			Error string `json:"error"`
		}

		resBody := returnVals {
			Error: "Something went wrong",
		} 

		data, err := json.Marshal(resBody)
		if err != nil {
			fmt.Printf("Error marshalling JSON: %s", err)
			response.WriteHeader(500)
			return
		}

		response.WriteHeader(400)
		response.Write(data)
		return
	}

	if len(requestData.Body) > 140 {
		type returnVals struct {
			Error string `json:"error"`
		}

		resBody := returnVals {
			Error: "Chirp is too long",
		} 

		data, err := json.Marshal(resBody)
		if err != nil {
			fmt.Printf("Error marshalling JSON: %s", err)
			response.WriteHeader(500)
			return
		}

		response.WriteHeader(400)
		response.Write(data)
		return
	}

	Cleaned_body := strings.Clone(requestData.Body)

	for _, word := range strings.Split(Cleaned_body, " ") {
		if( strings.ToLower(word) == "kerfuffle" || strings.ToLower(word) == "sharbert" || strings.ToLower(word) == "fornax" ){
			Cleaned_body = strings.ReplaceAll(Cleaned_body, word, "****")
		}
	}

	fmt.Println(Cleaned_body,"===================")
	
	type returnVals struct {
		Cleaned_body string `json:"cleaned_body"`
		Valid bool `json:"valid"`
	}

	resBody := returnVals {
		Cleaned_body: Cleaned_body,
		Valid: true,
	} 

	data, err := json.Marshal(resBody)
	if err != nil {
		fmt.Printf("Error marshalling JSON: %s", err)
		response.WriteHeader(500)
		return
	}

	response.WriteHeader(200)
	response.Write(data)
}

func (cfg *apiConfig) resetCountHandler(response http.ResponseWriter, request *http.Request) {
	cfg.fileserverHits.Store(0)
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
	mux := http.NewServeMux()
	cfg := apiConfig{}
	mux.Handle("/app/", http.StripPrefix("/app/", cfg.middlewareMetricsInc(http.FileServer(http.Dir("app")))))
	mux.HandleFunc("GET /api/healthz", healthzHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.requestCountHandler)
	mux.HandleFunc("POST /admin/reset", cfg.resetCountHandler)
	mux.HandleFunc("POST /api/validate_chirp", validateChirpHandler)
	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}

	fmt.Println("Hello how are you?")
	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("%v\n", err)
	}
}
