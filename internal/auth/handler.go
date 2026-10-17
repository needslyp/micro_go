package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Handler handles HTTP requests for the Auth service.
type Handler struct {
	repo *UserRepository
}

// Register registers a new user.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// После первого JSON не должно быть других данных.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "expected one JSON object")
		return
	}

	email := strings.ToLower(strings.TrimSpace(request.Email))
	if err := checkEmail(email); err != nil {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}

	if utf8.RuneCountInString(request.Password) < 8 || len(request.Password) > 72 {
		writeError(w, http.StatusBadRequest, "invalid password")
		return
	}

	hash, err := hashPassword(request.Password)
	if err != nil {
		log.Printf("error hashing password: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user, err := h.repo.Create(r.Context(), email, hash)
	if errors.Is(err, ErrEmailTaken) {
		writeError(w, http.StatusConflict, "email already taken")
		return
	}
	if err != nil {
		log.Printf("error creating user: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, userResponse{ID: user.ID, Email: user.Email, CreatedAt: user.CreatedAt})

}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})

}
