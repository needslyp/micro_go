package blog

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

// Handler handles HTTP requests for the Auth service.
type Handler struct {
	service *Service
}

type articleListResponse struct {
	Items  []articleResponse `json:"items"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

func toArticleListResponse(article Article) articleResponse {
	return articleResponse{
		ID:        article.ID,
		AuthorID:  article.AuthorID,
		Title:     article.Title,
		Body:      article.Body,
		CreatedAt: article.CreatedAt.UTC(),
		UpdatedAt: article.UpdatedAt.UTC(),
	}
}

// GetByID handle function to find article by ID
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid article id")
		return
	}

	article, err := h.service.GetArticleByID(r.Context(), id)

	switch {
	case errors.Is(err, ErrInvalidArticleID):
		writeError(w, http.StatusBadRequest, "invalid article id")
	case errors.Is(err, ErrArticleNotFound):
		writeError(w, http.StatusNotFound, "article not found")
	case err != nil:
		log.Printf("GetByID: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	default:
		writeJSON(w, http.StatusOK, toArticleListResponse(article))
	}
}

func readPagination(r *http.Request) (offset, limit int, err error) {
	offset, limit = 0, 20

	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, 0, ErrInvalidPagination
	}

	for key := range query {
		if key != "limit" && key != "offset" {
			return 0, 0, ErrInvalidPagination
		}
	}

	if values, ok := query["offset"]; ok {
		if len(values) != 1 {
			return 0, 0, ErrInvalidPagination
		}
		offset, err = strconv.Atoi(values[0])
		if err != nil {
			return 0, 0, ErrInvalidPagination
		}
	}

	if values, ok := query["limit"]; ok {
		if len(values) != 1 {
			return 0, 0, ErrInvalidPagination
		}
		limit, err = strconv.Atoi(values[0])
		if err != nil {
			return 0, 0, ErrInvalidPagination
		}
	}
	return offset, limit, nil
}

// GetList handle function to find all articles
func (h *Handler) GetList(w http.ResponseWriter, r *http.Request) {
	offset, limit, err := readPagination(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
	}

	articles, err := h.service.GetList(r.Context(), limit, offset)
	if errors.Is(err, ErrInvalidPagination) {
		writeError(w, http.StatusBadRequest, "invalid pagination")
		return
	}
	if err != nil {
		log.Printf("GetList: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	items := make([]articleResponse, 0, len(articles))
	for _, article := range articles {
		items = append(items, toArticleListResponse(article))
	}

	writeJSON(w, http.StatusOK, articleListResponse{Items: items, Limit: limit, Offset: offset})
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
