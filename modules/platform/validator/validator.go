package validator

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type AppValidator struct {
	validate *validator.Validate
}

func NewAppValidator() *AppValidator {
	v := validator.New()
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return &AppValidator{validate: v}
}

// ValidateStruct validates a DTO and maps internal errors into readable API responses
func (av *AppValidator) ValidateStruct(s interface{}) []ValidationError {
	err := av.validate.Struct(s)
	if err == nil {
		return nil
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		out := make([]ValidationError, len(ve))
		for i, fe := range ve {
			out[i] = ValidationError{
				Field:   fe.Field(),
				Message: msgForTag(fe.Tag(), fe.Param()),
			}
		}
		return out
	}

	// Fallback for non-validation errors (e.g., nil pointer passed to validator)
	return []ValidationError{{Field: "system", Message: "Invalid payload wrapper"}}
}

// msgForTag converts validation failure tags into user-friendly messages
func msgForTag(tag string, param string) string {
	switch tag {
	case "required":
		return "This field is required."
	case "alpha":
		return "This field must contain only alphabetic characters."
	case "email":
		return "This must be a valid email address."
	case "e164":
		return "Must be a valid international phone number format (e.g., +1234567890)."
	case "min":
		return fmt.Sprintf("Must be at least %s characters long.", param)
	case "max":
		return fmt.Sprintf("Cannot be longer than %s characters.", param)
	default:
		return "This field is invalid."
	}
}
