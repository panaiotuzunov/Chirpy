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
	"github.com/panaiotuzunov/Chirpy/internal/auth"
	"github.com/panaiotuzunov/Chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	secret         string
}
type errorResponse struct {
	Error string `json:"error"`
}
type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
}
type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

const maxChirpLength int = 140
const accessTokenExpiration = time.Hour
const refreshTokenExpiration = time.Hour * 60 * 24

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
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&requestData); err != nil {
		log.Printf("Error decoding JSON: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error decoding JSON")
		return
	}
	hashedPassword, err := auth.HashPassword(requestData.Password)
	if err != nil {
		log.Printf("Error hashing password - %v", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error creating user.")
		return
	}
	params := database.CreateUserParams{Email: requestData.Email, HashedPassword: hashedPassword}
	userResult, err := cfg.db.CreateUser(req.Context(), params)
	if err != nil {
		log.Printf("Error creating user - %v", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error creating user.")
		return
	}
	writeJSONResponse(writer, http.StatusCreated, User{
		ID:        userResult.ID,
		CreatedAt: userResult.CreatedAt,
		UpdatedAt: userResult.UpdatedAt,
		Email:     userResult.Email})
}

func (cfg *apiConfig) handlerLogin(writer http.ResponseWriter, req *http.Request) {
	var requestData struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&requestData); err != nil {
		log.Printf("Error decoding JSON: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error decoding JSON")
		return
	}
	user, err := cfg.db.GetUserByEmail(req.Context(), requestData.Email)
	if err != nil {
		log.Printf("Error getting user from DB: %s", err)
		writeErrorResponse(writer, http.StatusUnauthorized, "Incorrect email or password")
		return
	}
	if err := auth.CheckPasswordHash(requestData.Password, user.HashedPassword); err != nil {
		writeErrorResponse(writer, http.StatusUnauthorized, "incorrect email or password")
		return
	}
	token, err := auth.MakeJWT(user.ID, cfg.secret, accessTokenExpiration)
	if err != nil {
		log.Printf("Error creating token: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Server Error.")
		return
	}
	refreshTokenString, err := auth.MakeRefreshToken()
	if err != nil {
		fmt.Printf("Error creating token: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Server Error.")
		return
	}
	refreshToken, err := cfg.db.CreateRefreshToken(req.Context(), database.CreateRefreshTokenParams{
		Token:     refreshTokenString,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(refreshTokenExpiration),
	})
	if err != nil {
		log.Printf("Error creating refresh token in DB: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "DB Server Error.")
		return
	}
	writeJSONResponse(writer, http.StatusOK, User{
		ID:           user.ID,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
		Email:        user.Email,
		Token:        token,
		RefreshToken: refreshToken.Token,
	})
}

func (cfg *apiConfig) handlerAddChirp(writer http.ResponseWriter, req *http.Request) {
	var requestData struct {
		Body string `json:"body"`
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
	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		log.Printf("Error getting token: %s", err)
		writeErrorResponse(writer, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		log.Printf("Error validating token: %s", err)
		writeErrorResponse(writer, http.StatusUnauthorized, "Invalid token")
		return
	}
	chirp, err := cfg.db.CreateChirp(req.Context(), database.CreateChirpParams{Body: hideProfanity(requestData.Body), UserID: id})
	if err != nil {
		log.Printf("Error decoding JSON: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error creating chirp")
		return
	}
	writeJSONResponse(writer, http.StatusCreated, Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID})
}

func (cfg *apiConfig) handlerChirps(writer http.ResponseWriter, req *http.Request) {
	chirps, err := cfg.db.GetChirps(req.Context())
	if err != nil {
		log.Printf("Error getting chirps from DB: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error getting chirps")
		return
	}
	var resultChirps []Chirp
	for _, chirp := range chirps {
		resultChirps = append(resultChirps, Chirp{ID: chirp.ID, CreatedAt: chirp.CreatedAt, UpdatedAt: chirp.UpdatedAt, Body: chirp.Body, UserID: chirp.UserID})
	}
	writeJSONResponse(writer, http.StatusOK, resultChirps)
}

func (cfg *apiConfig) handlerGetChirp(writer http.ResponseWriter, req *http.Request) {
	id, err := uuid.Parse(req.PathValue("chirpID"))
	if err != nil {
		log.Printf("Error parsing chirpID argument: %s", err)
		writeErrorResponse(writer, http.StatusBadRequest, "Invalid ID")
		return
	}
	chirp, err := cfg.db.GetChirpByID(req.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeErrorResponse(writer, http.StatusNotFound, "Not found")
			return
		}
		log.Printf("Error getting chirp from DB: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error getting chirp")
		return
	}
	writeJSONResponse(writer, http.StatusOK, Chirp{ID: chirp.ID, CreatedAt: chirp.CreatedAt, UpdatedAt: chirp.UpdatedAt, Body: chirp.Body, UserID: chirp.UserID})
}

func (cfg *apiConfig) handlerRefresh(writer http.ResponseWriter, req *http.Request) {
	tokenString, err := auth.GetBearerToken(req.Header)
	if err != nil {
		log.Printf("Error getting bearer token: %s", err)
		writeErrorResponse(writer, http.StatusBadRequest, "Invalid header")
		return
	}
	refreshToken, err := cfg.db.GetUserFromRefreshToken(req.Context(), tokenString)
	if err != nil {
		log.Printf("Error getting user from refresh token: %s", err)
		writeErrorResponse(writer, http.StatusUnauthorized, "Invalid token")
		return
	}
	if time.Now().After(refreshToken.ExpiresAt) {
		log.Println("Refresh token expired")
		writeErrorResponse(writer, http.StatusUnauthorized, "Token expired.")
		return
	}
	if refreshToken.RevokedAt.Valid {
		log.Println("Refresh token revoked")
		writeErrorResponse(writer, http.StatusUnauthorized, "Token revoked.")
		return
	}
	jwt, err := auth.MakeJWT(refreshToken.UserID, cfg.secret, accessTokenExpiration)
	if err != nil {
		log.Printf("Error creating JWT - %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Server error")
		return
	}
	writeJSONResponse(writer, http.StatusOK, struct {
		Token string `json:"token"`
	}{
		Token: jwt,
	})
}

func (cfg *apiConfig) handlerRevoke(writer http.ResponseWriter, req *http.Request) {
	tokenString, err := auth.GetBearerToken(req.Header)
	if err != nil {
		log.Printf("Error getting bearer token: %s", err)
		writeErrorResponse(writer, http.StatusBadRequest, "Invalid header")
		return
	}
	if err := cfg.db.RevokeRefreshToken(req.Context(), tokenString); err != nil {
		log.Printf("Error revoking token: %s", err)
		writeErrorResponse(writer, http.StatusUnauthorized, "Invalid token")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handlerUpdateCredentials(writer http.ResponseWriter, req *http.Request) {
	var requestData struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	tokenString, err := auth.GetBearerToken(req.Header)
	if err != nil {
		log.Printf("Error getting bearer token: %s", err)
		writeErrorResponse(writer, http.StatusUnauthorized, "Missing or invalid token")
		return
	}
	userID, err := auth.ValidateJWT(tokenString, cfg.secret)
	if err != nil {
		log.Printf("Error validating access token: %s", err)
		writeErrorResponse(writer, http.StatusUnauthorized, "Missing or invalid token")
		return
	}
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&requestData); err != nil {
		log.Printf("Error decoding JSON: %s", err)
		writeErrorResponse(writer, http.StatusBadRequest, "Error decoding JSON")
		return
	}
	hashedPassword, err := auth.HashPassword(requestData.Password)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Server error")
		return
	}
	user, err := cfg.db.UpdateCredentials(req.Context(), database.UpdateCredentialsParams{
		Email:          requestData.Email,
		HashedPassword: hashedPassword,
		ID:             userID,
	})
	if err != nil {
		log.Printf("Error updating credentials: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "DB Server error")
		return
	}
	writeJSONResponse(writer, http.StatusOK, User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
		Token:     tokenString,
	})
}

func (cfg *apiConfig) handlerDeleteChirp(writer http.ResponseWriter, req *http.Request) {
	tokenString, err := auth.GetBearerToken(req.Header)
	if err != nil {
		log.Printf("Error getting bearer token: %s", err)
		writeErrorResponse(writer, http.StatusUnauthorized, "Missing or invalid token")
		return
	}
	userID, err := auth.ValidateJWT(tokenString, cfg.secret)
	if err != nil {
		log.Printf("Error validating access token: %s", err)
		writeErrorResponse(writer, http.StatusUnauthorized, "Missing or invalid token")
		return
	}
	chirpID, err := uuid.Parse(req.PathValue("chirpID"))
	if err != nil {
		log.Printf("Error parsing chirpID argument: %s", err)
		writeErrorResponse(writer, http.StatusBadRequest, "Invalid ID")
		return
	}
	chirp, err := cfg.db.GetChirpByID(req.Context(), chirpID)
	if err != nil {
		log.Printf("Chirp not found: %s", err)
		writeErrorResponse(writer, http.StatusNotFound, "No chirp found")
		return
	}
	if chirp.UserID != userID {
		writeErrorResponse(writer, http.StatusForbidden, "Forbidden")
		return
	}
	if err := cfg.db.DeleteChirp(req.Context(), chirpID); err != nil {
		log.Printf("Error deleting chirp: %s", err)
		writeErrorResponse(writer, http.StatusInternalServerError, "Error deleting chirp")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
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
	cfg := apiConfig{
		db:       database.New(db),
		platform: os.Getenv("PLATFORM"),
		secret:   os.Getenv("SECRET"),
	}
	mux := http.NewServeMux()
	server := http.Server{
		Handler: mux,
		Addr:    ":8080",
	}
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir("./")))))
	mux.HandleFunc("GET /admin/metrics", cfg.ReturnMetrics)
	mux.HandleFunc("POST /admin/reset", cfg.Reset)
	mux.HandleFunc("GET /api/healthz", handlerHealthz)
	mux.HandleFunc("GET /api/chirps", cfg.handlerChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.handlerGetChirp)
	mux.HandleFunc("POST /api/chirps", cfg.handlerAddChirp)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", cfg.handlerDeleteChirp)
	mux.HandleFunc("POST /api/users", cfg.handlerCreateUser)
	mux.HandleFunc("PUT /api/users", cfg.handlerUpdateCredentials)
	mux.HandleFunc("POST /api/login", cfg.handlerLogin)
	mux.HandleFunc("POST /api/refresh", cfg.handlerRefresh)
	mux.HandleFunc("POST /api/revoke", cfg.handlerRevoke)
	log.Print("Server is running")
	server.ListenAndServe()
}
