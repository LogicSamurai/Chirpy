package main

import (
	"fmt"
	"net/http"
)

func healthzHandler(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	response.WriteHeader(200)
	response.Write([]byte("OK"))

}

func main() {
	mux := http.NewServeMux()
	mux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("app"))))

	mux.HandleFunc("GET /healthz", healthzHandler)
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
