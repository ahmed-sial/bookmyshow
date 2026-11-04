package identity

import (
	"time"

	"github.com/google/uuid"
)

type user struct {
	createdAt    time.Time
	updatedAt    time.Time
	deletedAt    time.Time
	firstName    string
	lastName     string
	email        string
	passwordHash string
	phoneNumber  string
	id           uuid.UUID
	status       bool
}
