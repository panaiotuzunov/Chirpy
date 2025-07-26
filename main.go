package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/panaiotuzunov/Chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
}
type errorResponse struct {
	Error string `json:"error"`
}
type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}
type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

const maxChirpLength int = 140

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

func (cfg *apiConfig) Reset(writer http.ResponseWriter, req *http.Request) {
	if cfg.platform != "dev" {
		writeErrorResponse(writer, http.StatusForbidden, "Forbidden")
		return
	}
	if err := cfg.db.DeleteUsers(req.Context()); err != nil {
		log.Printf("Error deleting users - %v", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Reset failed")
		return
	}
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

func (cfg *apiConfig) handlerCreateUser(writer http.ResponseWriter, req *http.Request) {
	var requestData struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&requestData); err != nil {
		log.Printf("Error decoding JSON: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error decoding JSON")
		return
	}
	userResult, err := cfg.db.CreateUser(req.Context(), requestData.Email)
	if err != nil {
		log.Printf("Error creating user - %v", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error creating user.")
		return
	}
	writeJSONResponse(writer, http.StatusCreated, User{ID: userResult.ID, CreatedAt: userResult.CreatedAt, UpdatedAt: userResult.UpdatedAt, Email: userResult.Email})
}

func (cfg *apiConfig) handlerChirps(writer http.ResponseWriter, req *http.Request) {
	var requestData struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&requestData); err != nil {
		log.Printf("Error decoding JSON: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error decoding JSON")
		return
	}
	if len(requestData.Body) > maxChirpLength {
		writeErrorResponse(writer, http.StatusBadRequest, "Chirp is too long")
		return
	}
	chirp, err := cfg.db.CreateChirp(req.Context(), database.CreateChirpParams{Body: hideProfanity(requestData.Body), UserID: requestData.UserID})
	if err != nil {
		log.Printf("Error decoding JSON: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error creating chirp")
		return
	}
	writeJSONResponse(writer, http.StatusCreated, Chirp{ID: chirp.ID, CreatedAt: chirp.CreatedAt, UpdatedAt: chirp.UpdatedAt, Body: chirp.Body, UserID: chirp.UserID})
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
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
func writeErrorResponse(w http.ResponseWriter, statusCode int, text string) {
	writeJSONResponse(w, statusCode, errorResponse{Error: text})
}

func hideProfanity(chirp string) string {
	profaneWords := map[string]struct{}{
		"kerfuffle": {},
		"sharbert":  {},
		"fornax":    {},
	}
	var resultSlice []string
	words := strings.SplitSeq(chirp, " ")
	for word := range words {
		_, isProfane := profaneWords[strings.ToLower(word)]
		if isProfane {
			resultSlice = append(resultSlice, "****")
			continue
		}
		resultSlice = append(resultSlice, word)
	}
	return strings.Join(resultSlice, " ")
}

func main() {
	godotenv.Load(".env")
	db, err := sql.Open("postgres", os.Getenv("DB_URL"))
	if err != nil {
		log.Fatalf("Error connecting to DB - %v", err)
	}
	cfg := apiConfig{db: database.New(db), platform: os.Getenv("PLATFORM")}
	mux := http.NewServeMux()
	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir("./")))))
	mux.HandleFunc("GET /api/healthz", handlerHealthz)
	mux.HandleFunc("GET /admin/metrics", cfg.ReturnMetrics)
	mux.HandleFunc("POST /admin/reset", cfg.Reset)
	mux.HandleFunc("POST /api/chirps", cfg.handlerChirps)
	mux.HandleFunc("POST /api/users", cfg.handlerCreateUser)
	log.Print("Server is running")
	server.ListenAndServe()
}
