package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SystemOpsSubject = "system:ops"

type Users struct {
	pool *pgxpool.Pool
}

func NewUsers(pool *pgxpool.Pool) *Users {
	return &Users{pool: pool}
}

type User struct {
	ID            string
	AuthSubject   string
	Email         string
	EmailVerified bool
	DisplayName   string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastLoginAt   *time.Time
}

type UpsertUserInput struct {
	AuthSubject   string
	Email         string
	EmailVerified bool
	DisplayName   string
}

func (s *Users) UpsertBySubject(ctx context.Context, in UpsertUserInput) (User, error) {
	if s == nil || s.pool == nil {
		return User{}, fmt.Errorf("store not configured")
	}
	if in.AuthSubject == "" {
		return User{}, fmt.Errorf("auth_subject is required")
	}

	if existing, err := s.GetBySubject(ctx, in.AuthSubject); err == nil {
		if existing.Status != "active" {
			return existing, nil
		}
		return s.updateActive(ctx, existing.ID, in)
	}

	var email any
	if in.Email != "" {
		email = in.Email
	}
	var display any
	if in.DisplayName != "" {
		display = in.DisplayName
	}
	var u User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (auth_subject, email, email_verified, display_name, last_login_at, updated_at)
		VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (auth_subject) DO UPDATE SET
			email = COALESCE(EXCLUDED.email, users.email),
			email_verified = users.email_verified OR EXCLUDED.email_verified,
			display_name = COALESCE(EXCLUDED.display_name, users.display_name),
			last_login_at = CASE WHEN users.status = 'active' THEN now() ELSE users.last_login_at END,
			updated_at = CASE WHEN users.status = 'active' THEN now() ELSE users.updated_at END
		RETURNING id::text, auth_subject, COALESCE(email, ''), email_verified,
			COALESCE(display_name, ''), status, created_at, updated_at, last_login_at
	`, in.AuthSubject, email, in.EmailVerified, display).Scan(
		&u.ID,
		&u.AuthSubject,
		&u.Email,
		&u.EmailVerified,
		&u.DisplayName,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.LastLoginAt,
	)
	if err != nil {
		return User{}, fmt.Errorf("upsert user: %w", err)
	}
	return u, nil
}

func (s *Users) updateActive(ctx context.Context, id string, in UpsertUserInput) (User, error) {
	var email any
	if in.Email != "" {
		email = in.Email
	}
	var display any
	if in.DisplayName != "" {
		display = in.DisplayName
	}
	var u User
	err := s.pool.QueryRow(ctx, `
		UPDATE users SET
			email = COALESCE($2, email),
			email_verified = email_verified OR $3,
			display_name = COALESCE($4, display_name),
			last_login_at = now(),
			updated_at = now()
		WHERE id = $1 AND status = 'active'
		RETURNING id::text, auth_subject, COALESCE(email, ''), email_verified,
			COALESCE(display_name, ''), status, created_at, updated_at, last_login_at
	`, id, email, in.EmailVerified, display).Scan(
		&u.ID,
		&u.AuthSubject,
		&u.Email,
		&u.EmailVerified,
		&u.DisplayName,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.LastLoginAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.GetByID(ctx, id)
	}
	if err != nil {
		return User{}, fmt.Errorf("update user: %w", err)
	}
	return u, nil
}

func (s *Users) GetByID(ctx context.Context, id string) (User, error) {
	if s == nil || s.pool == nil {
		return User{}, fmt.Errorf("store not configured")
	}
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, auth_subject, COALESCE(email, ''), email_verified,
			COALESCE(display_name, ''), status, created_at, updated_at, last_login_at
		FROM users WHERE id = $1
	`, id).Scan(
		&u.ID,
		&u.AuthSubject,
		&u.Email,
		&u.EmailVerified,
		&u.DisplayName,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.LastLoginAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("user not found")
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (s *Users) GetBySubject(ctx context.Context, subject string) (User, error) {
	if s == nil || s.pool == nil {
		return User{}, fmt.Errorf("store not configured")
	}
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, auth_subject, COALESCE(email, ''), email_verified,
			COALESCE(display_name, ''), status, created_at, updated_at, last_login_at
		FROM users WHERE auth_subject = $1
	`, subject).Scan(
		&u.ID,
		&u.AuthSubject,
		&u.Email,
		&u.EmailVerified,
		&u.DisplayName,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
		&u.LastLoginAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("user not found")
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by subject: %w", err)
	}
	return u, nil
}

// EnsureSystemOps returns the bootstrap ops user (created by migration).
func (s *Users) EnsureSystemOps(ctx context.Context) (User, error) {
	u, err := s.GetBySubject(ctx, SystemOpsSubject)
	if err == nil {
		return u, nil
	}
	return s.UpsertBySubject(ctx, UpsertUserInput{
		AuthSubject: SystemOpsSubject,
		DisplayName: "System Ops",
	})
}
