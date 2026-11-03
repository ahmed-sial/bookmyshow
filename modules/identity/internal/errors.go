package identity

import (
	"errors"

	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

var ErrEmailTaken = errors.New("email already registered")
var ErrPhoneTaken = errors.New("phone number already registered")
var ErrUserNotFound = errors.New("user not found")
var ErrEmailNotFound = errors.New("email not found")
var ErrUnauthorized = errors.New("unauthorized")

type ValidationError struct {
	Fields []validator.FieldError
}

func (e *ValidationError) Error() string { return "validation failed" }
