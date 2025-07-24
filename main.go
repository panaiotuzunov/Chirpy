package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) ReturnMetrics(writer http.ResponseWriter, req *http.Request) {
	result := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())
	writer.Header().Add("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte(result))
}

func (cfg *apiConfig) ResetMetrics(writer http.ResponseWriter, req *http.Request) {
	cfg.fileserverHits.Store(0)
	writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}

func handlerHealthz(writer http.ResponseWriter, req *http.Request) {
	writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}

func handlerValidateChirp(writer http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Body  string `json:"body"`
		Error string `json:"error"`
		Valid bool   `json:"valid"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		log.Printf("Error decoding parameters: %s", err)
		writer.Header().Add("Content-Type", "application/json")
		writer.WriteHeader(http.StatusInternalServerError)
		respBody := parameters{
			Error: "Error decoding request",
		}
		data, err := json.Marshal(respBody)
		if err != nil {
			log.Printf("Error encoding JSON: %s", err)
			return
		}
		writer.Write(data)
		return
	}
	if len(params.Body) > 140 {
		respBody := parameters{
			Error: "Chirp exceeds the maximum length of 140 characters",
		}
		data, err := json.Marshal(respBody)
		if err != nil {
			log.Printf("Error encoding JSON: %s", err)
			return
		}
		writer.WriteHeader(400)
		writer.Write(data)
		return
	}
	respBody := parameters{
		Valid: true,
	}
	data, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error encoding JSON: %s", err)
		return
	}
	writer.Write(data)
	writer.Header().Add("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
}

func main() {
	cfg := apiConfig{}
	mux := http.NewServeMux()
	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir("./")))))
	mux.HandleFunc("GET /api/healthz", handlerHealthz)
	mux.HandleFunc("GET /admin/metrics", cfg.ReturnMetrics)
	mux.HandleFunc("POST /admin/reset", cfg.ResetMetrics)
	mux.HandleFunc("POST /api/validate_chirp", handlerValidateChirp)
	server.ListenAndServe()
}
