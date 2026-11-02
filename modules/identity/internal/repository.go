package identity

import (
	"context"
	"database/sql"
)

type IdentityRepository interface {
	Create(ctx context.Context, user user) error
	FindByID(ctx context.Context, id string) (*user, error)
	FindAll(ctx context.Context) ([]*user, error)
	Update(ctx context.Context, user *user) error
	Delete(ctx context.Context, id string) error
}

type PostgresIdentityRepository struct {
	db *sql.DB
}

func NewPostgresIdentityRepository(db *sql.DB) *PostgresIdentityRepository {
	return &PostgresIdentityRepository{db: db}
}

func (p *PostgresIdentityRepository) Create(ctx context.Context, u user) error {
	query := `
		INSERT INTO users (first_name, last_name, email, password_hash, phone_number)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := p.db.ExecContext(ctx, query, u.firstName, u.lastName, u.email, u.passwordHash, u.phoneNumber, u.status)
	if err != nil {
		return err
	}
	return nil
}
