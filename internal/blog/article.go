// Package blog implements the articles service and its PostgreSQL repository.
package blog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Article holds article data.
type Article struct {
	ID        int64
	AuthorID  int64
	Title     string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type articleResponse struct {
	ID        int64     `json:"id"`
	AuthorID  int64     `json:"author_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ArticleRepository stores and retrieves articles in PostgreSQL.
type ArticleRepository struct {
	pool *pgxpool.Pool
}

const repositoryQueryTimeout = 3 * time.Second

var (
	// ErrArticleNotFound indicates that the article is not found.
	ErrArticleNotFound = errors.New("article not found")
	// ErrForbidden indicates that the user does not own the article.
	ErrForbidden = errors.New("forbidden")
)

// Create stores a new article for the given author.
func (r *ArticleRepository) Create(ctx context.Context, authorID int64, title, body string) (Article, error) {
	ctx, cancel := context.WithTimeout(ctx, repositoryQueryTimeout)
	defer cancel()

	var article Article
	err := r.pool.QueryRow(ctx, `
		INSERT INTO blog.articles (author_id, title, body)
		VALUES ($1, $2, $3)
		RETURNING id, author_id, title, body, created_at, updated_at;`,
		authorID, title, body).Scan(&article.ID, &article.AuthorID, &article.Title, &article.Body, &article.CreatedAt, &article.UpdatedAt)

	if err != nil {
		return Article{}, fmt.Errorf("create article: %w", err)
	}

	return article, nil
}

// GetByID finds an article by ID or returns ErrArticleNotFound.
func (r *ArticleRepository) GetByID(ctx context.Context, id int64) (Article, error) {
	ctx, cancel := context.WithTimeout(ctx, repositoryQueryTimeout)
	defer cancel()

	var article Article
	err := r.pool.QueryRow(ctx,
		`SELECT id, author_id, title, body, created_at, updated_at 
		FROM blog.articles WHERE id = $1;`,
		id).Scan(&article.ID, &article.AuthorID, &article.Title, &article.Body, &article.CreatedAt, &article.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return Article{}, ErrArticleNotFound
	}
	if err != nil {
		return Article{}, fmt.Errorf("get article: %w", err)
	}

	return article, nil
}

// GetList returns a page of articles ordered by creation time and ID, newest first.
func (r *ArticleRepository) GetList(ctx context.Context, limit int, offset int) ([]Article, error) {
	ctx, cancel := context.WithTimeout(ctx, repositoryQueryTimeout)
	defer cancel()

	rows, err := r.pool.Query(ctx, `
		SELECT id, author_id, title, body, created_at, updated_at
		FROM blog.articles
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2;`, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("get articles: %w", err)
	}
	defer rows.Close()

	articles := make([]Article, 0)
	for rows.Next() {
		var article Article
		if err := rows.Scan(&article.ID, &article.AuthorID, &article.Title, &article.Body, &article.CreatedAt, &article.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan articles: %w", err)
		}

		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read articles: %w", err)
	}

	return articles, nil
}

// Update replaces an article's title and body if authorID matches its owner.
func (r *ArticleRepository) Update(ctx context.Context, id, authorID int64, title, body string) (Article, error) {
	ctx, cancel := context.WithTimeout(ctx, repositoryQueryTimeout)
	defer cancel()

	var article Article
	err := r.pool.QueryRow(ctx, `
		UPDATE blog.articles
		SET title = $1, body = $2, updated_at = now()
		WHERE id = $3 AND author_id = $4
		RETURNING id, author_id, title, body, created_at, updated_at`,
		title, body, id, authorID).Scan(
		&article.ID, &article.AuthorID, &article.Title, &article.Body,
		&article.CreatedAt, &article.UpdatedAt)
	if err == nil {
		return article, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Article{}, fmt.Errorf("update article: %w", err)
	}

	var belongsToAnotherAuthor bool
	err = r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM blog.articles WHERE id = $1 AND author_id <> $2
		)`, id, authorID).Scan(&belongsToAnotherAuthor)
	if err != nil {
		return Article{}, fmt.Errorf("check article ownership: %w", err)
	}
	if belongsToAnotherAuthor {
		return Article{}, ErrForbidden
	}
	return Article{}, ErrArticleNotFound
}

// Delete removes an article if authorID matches its owner.
func (r *ArticleRepository) Delete(ctx context.Context, id, authorID int64) error {
	ctx, cancel := context.WithTimeout(ctx, repositoryQueryTimeout)
	defer cancel()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete article: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), repositoryQueryTimeout)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	var ownerID int64
	err = tx.QueryRow(ctx, `SELECT author_id FROM blog.articles WHERE id = $1 FOR UPDATE`, id).Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrArticleNotFound
	}
	if err != nil {
		return fmt.Errorf("lock article for delete: %w", err)
	}
	if ownerID != authorID {
		return ErrForbidden
	}
	if _, err := tx.Exec(ctx, `DELETE FROM blog.articles WHERE id = $1 AND author_id = $2`, id, authorID); err != nil {
		return fmt.Errorf("delete article: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete article: %w", err)
	}
	return nil
}
