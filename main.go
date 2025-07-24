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
	type requestBody struct {
		Body string `json:"body"`
	}

	type errorResponse struct {
		Error string `json:"error"`
	}

	type validResponse struct {
		Valid bool `json:"valid"`
	}

	decoder := json.NewDecoder(req.Body)
	params := requestBody{}
	if err := decoder.Decode(&params); err != nil {
		log.Printf("Error decoding JSON: %s", err)
		writeJSONResponse(writer, 500, errorResponse{Error: "Error decoding JSON"})
		return
	}
	if len(params.Body) > 140 {
		writeJSONResponse(writer, 400, errorResponse{Error: "Chirp is too long"})
		return
	}
	writeJSONResponse(writer, 200, validResponse{Valid: true})
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling JSON: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(jsonData)
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
