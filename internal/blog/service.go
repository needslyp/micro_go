package blog

import (
	"context"
	"errors"
)

var (
	// ErrInvalidArticleID implements invalid article ID.
	ErrInvalidArticleID = errors.New("invalid article id")
	// ErrInvalidPagination implements invalid pagination parameters.
	ErrInvalidPagination = errors.New("invalid pagination")
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
