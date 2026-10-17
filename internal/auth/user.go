package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrEmailTaken incdicates that the email is already registered.
	ErrEmailTaken = errors.New("email already exists")
	// ErrUserNotFound indicates that the user does not exist.
	ErrUserNotFound = errors.New("user not found")
)

// User holds user data.
type User struct {
	ID           int64
	Email        string
	CreatedAt    time.Time
	passwordHash string
}

type userResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func checkEmail(email string) error {
	if email == "" {
		return errors.New("empty email")
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" || address.Address != email {
		return errors.New("invalid email")
	}
	return nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return string(hash), nil
}

func checkPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// UserRepository stores and retrieves users in PostgreSQL.
type UserRepository struct {
	pool *pgxpool.Pool
}

// Create stores a new user with an already hashed password.
func (r *UserRepository) Create(ctx context.Context, email string, passwordHash string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	if err := checkEmail(email); err != nil {
		return User{}, err
	}
	if strings.TrimSpace(passwordHash) == "" {
		return User{}, errors.New("password hash is required")
	}

	var user User
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, password_hash, created_at`,
		email, passwordHash).Scan(
		&user.ID,
		&user.Email,
		&user.passwordHash,
		&user.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

// GetByEmail finds a user by email.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var user User
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, created_at
		FROM users
		WHERE email = $1`,
		email).Scan(
		&user.ID,
		&user.Email,
		&user.passwordHash,
		&user.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by email: %w", err)
	}

	return user, nil
}
