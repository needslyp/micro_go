package blog

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

// Handler handles HTTP requests for the Blog service.
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
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
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
		writeError(w, http.StatusBadRequest, "invalid pagination")
		return
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

type articleRequest struct {
	Title *string `json:"title"`
	Body  *string `json:"body"`
}

func readArticle(w http.ResponseWriter, r *http.Request) (articleRequest, bool) {
	var request articleRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&request)
	if err == nil {
		var extra any
		if nextErr := decoder.Decode(&extra); nextErr != io.EOF {
			err = nextErr
			if err == nil {
				err = errors.New("additional JSON value")
			}
		}
	}
	if err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON")
		}
		return request, false
	}
	if request.Title == nil || request.Body == nil {
		writeError(w, http.StatusBadRequest, "title and body are required")
		return request, false
	}
	return request, true
}

// Create stores an article for the authenticated user.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	authorID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid access token")
		return
	}
	request, ok := readArticle(w, r)
	if !ok {
		return
	}
	article, err := h.service.Create(r.Context(), authorID, *request.Title, *request.Body)
	if err != nil {
		writeArticleError(w, err)
		return
	}
	w.Header().Set("Location", "/articles/"+strconv.FormatInt(article.ID, 10))
	writeJSON(w, http.StatusCreated, toArticleListResponse(article))
}

// Update replaces the title and body of an article owned by the authenticated user.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	authorID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid access token")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid article id")
		return
	}
	request, ok := readArticle(w, r)
	if !ok {
		return
	}
	article, err := h.service.Update(r.Context(), id, authorID, *request.Title, *request.Body)
	if err != nil {
		writeArticleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toArticleListResponse(article))
}

// Delete removes an article owned by the authenticated user.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	authorID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid access token")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid article id")
		return
	}
	if err := h.service.Delete(r.Context(), id, authorID); err != nil {
		writeArticleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeArticleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidArticleID), errors.Is(err, ErrInvalidAuthorID),
		errors.Is(err, ErrInvalidTitle), errors.Is(err, ErrInvalidBody):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrArticleNotFound):
		writeError(w, http.StatusNotFound, "article not found")
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	default:
		log.Printf("write article: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
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
