package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type IdentityRepository interface {
	Create(ctx context.Context, u user) error
	FindByID(ctx context.Context, id uuid.UUID) (*user, error)
	FindByEmail(ctx context.Context, e string) (*user, error)
	ExistsByEmail(ctx context.Context, e string) (bool, error)
}

type PostgresIdentityRepository struct {
	db *sql.DB
}

func NewPostgresIdentityRepository(db *sql.DB) *PostgresIdentityRepository {
	return &PostgresIdentityRepository{db: db}
}

func (p *PostgresIdentityRepository) Create(ctx context.Context, u user) error {
	const query = `
		INSERT INTO users (id, first_name, last_name, email, password_hash, phone_number, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := p.db.ExecContext(
		ctx,
		query,
		u.id,
		u.firstName,
		nullIfEmpty(u.lastName),
		u.email,
		u.passwordHash,
		u.phoneNumber,
		u.status,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			switch pgErr.ConstraintName {
			case "users_email_key":
				return ErrEmailTaken
			case "users_phone_number_key":
				return ErrPhoneTaken
			}
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (p *PostgresIdentityRepository) FindByID(ctx context.Context, id uuid.UUID) (*user, error) {
	const query = `
		SELECT id, first_name, last_name, email, phone_number
		FROM users
		WHERE id = $1
		AND deleted_at IS NULL
	`
	var u user
	var lastName sql.NullString
	result := p.db.QueryRowContext(ctx, query, id)
	err := result.Scan(&u.id, &u.firstName, &lastName, &u.email, &u.phoneNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	u.lastName = lastName.String
	return &u, nil
}

func (p *PostgresIdentityRepository) FindByEmail(ctx context.Context, e string) (*user, error) {
	const query = `
		SELECT id, first_name, last_name, email, phone_number, password_hash
		FROM users
		WHERE email = $1
		AND deleted_at IS NULL
	`
	var u user
	var lastName sql.NullString
	result := p.db.QueryRowContext(ctx, query, e)
	err := result.Scan(&u.id, &u.firstName, &lastName, &u.email, &u.phoneNumber, &u.passwordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	u.lastName = lastName.String
	return &u, nil
}

func (p *PostgresIdentityRepository) ExistsByEmail(ctx context.Context, e string) (bool, error) {
	query := `SELECT EXISTS (
		SELECT 1
		FROM users
		WHERE email = $1
	)`

	var exists bool
	err := p.db.QueryRowContext(ctx, query, e).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
