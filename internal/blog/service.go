package blog

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	// ErrInvalidArticleID implements invalid article ID.
	ErrInvalidArticleID = errors.New("invalid article id")
	// ErrInvalidPagination implements invalid pagination parameters.
	ErrInvalidPagination = errors.New("invalid pagination")
	// ErrInvalidAuthorID indicates that the author ID is not positive.
	ErrInvalidAuthorID = errors.New("invalid author id")
	// ErrInvalidTitle indicates that the title is empty or exceeds 200 code points.
	ErrInvalidTitle = errors.New("invalid title")
	// ErrInvalidBody indicates that the body is blank or exceeds 50000 code points.
	ErrInvalidBody = errors.New("invalid body")
)

// Service implements article working rules.
type Service struct {
	repo *ArticleRepository
}

// GetArticleByID returns an article by ID.
func (s *Service) GetArticleByID(ctx context.Context, id int64) (Article, error) {
	if id <= 0 {
		return Article{}, ErrInvalidArticleID
	}
	return s.repo.GetByID(ctx, id)
}

// GetList returns a list of the articles.
func (s *Service) GetList(ctx context.Context, limit, offset int) ([]Article, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalidPagination
	}

	return s.repo.GetList(ctx, limit, offset)
}

func validateArticle(title, body string) (string, error) {
	title = strings.TrimSpace(title)
	titleLength := utf8.RuneCountInString(title)
	if titleLength < 1 || titleLength > 200 {
		return "", ErrInvalidTitle
	}
	if strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > 50000 {
		return "", ErrInvalidBody
	}
	return title, nil
}

// Create validates article data and stores a new article.
func (s *Service) Create(ctx context.Context, authorID int64, title, body string) (Article, error) {
	if authorID <= 0 {
		return Article{}, ErrInvalidAuthorID
	}
	title, err := validateArticle(title, body)
	if err != nil {
		return Article{}, err
	}
	return s.repo.Create(ctx, authorID, title, body)
}

// Update validates article data and updates an article owned by authorID.
func (s *Service) Update(ctx context.Context, id, authorID int64, title, body string) (Article, error) {
	if id <= 0 {
		return Article{}, ErrInvalidArticleID
	}
	if authorID <= 0 {
		return Article{}, ErrInvalidAuthorID
	}
	title, err := validateArticle(title, body)
	if err != nil {
		return Article{}, err
	}
	return s.repo.Update(ctx, id, authorID, title, body)
}

// Delete removes an article owned by authorID.
func (s *Service) Delete(ctx context.Context, id, authorID int64) error {
	if id <= 0 {
		return ErrInvalidArticleID
	}
	if authorID <= 0 {
		return ErrInvalidAuthorID
	}
	return s.repo.Delete(ctx, id, authorID)
}
