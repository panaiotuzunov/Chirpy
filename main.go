package main

import (
	"fmt"
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
	result := fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())
	writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte(result))
}

func (cfg *apiConfig) ResetMetrics(writer http.ResponseWriter, req *http.Request) {
	cfg.fileserverHits.Store(0)
	writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}

func main() {
	cfg := apiConfig{}
	mux := http.NewServeMux()
	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir("./")))))
	mux.HandleFunc("/healthz", handlerHealthz)
	mux.HandleFunc("/metrics", cfg.ReturnMetrics)
	mux.HandleFunc("/reset", cfg.ResetMetrics)
	server.ListenAndServe()
}

func handlerHealthz(writer http.ResponseWriter, req *http.Request) {
	writer.Header().Add("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}
