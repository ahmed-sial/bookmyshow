package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

type IdentityRepository interface {
	Create(ctx context.Context, u user) error
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
