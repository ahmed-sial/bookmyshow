package validator

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
)

// phonePattern matches the users.phone_number CHECK constraint in the
// database migrations exactly, so values the API accepts can never be
// rejected by the schema (a mismatch would surface as a 500).
var phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

type FieldError struct {
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
	v.RegisterValidation("password", func(fl validator.FieldLevel) bool {
		var upper, lower, digit bool
		for _, r := range fl.Field().String() {
			switch {
			case unicode.IsUpper(r):
				upper = true
			case unicode.IsLower(r):
				lower = true
			case unicode.IsDigit(r):
				digit = true
			}
		}
		return upper && lower && digit
	})

	// Stricter than this validator's e164 rule, which accepts values the
	// database CHECK would reject (no leading "+", too few digits).
	v.RegisterValidation("phone", func(fl validator.FieldLevel) bool {
		return phonePattern.MatchString(fl.Field().String())
	})
	return &AppValidator{validate: v}
}

// ValidateStruct validates a DTO and maps internal errors into readable API responses
func (av *AppValidator) ValidateStruct(s any) []FieldError {
	err := av.validate.Struct(s)
	if err == nil {
		return nil
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		out := make([]FieldError, len(ve))
		for i, fe := range ve {
			out[i] = FieldError{
				Field:   fe.Field(),
				Message: msgForTag(fe.Tag(), fe.Param()),
			}
		}
		return out
	}

	// Fallback for non-validation errors (e.g., nil pointer passed to validator)
	return []FieldError{{Field: "system", Message: "Invalid payload wrapper"}}
}

// msgForTag converts validation failure tags into user-friendly messages
func msgForTag(tag string, param string) string {
	switch tag {
	case "required":
		return "This field is required."
	case "email":
		return "This must be a valid email address."
	case "phone":
		return "Must be a valid international phone number format (e.g., +1234567890)."
	case "min":
		return fmt.Sprintf("Must be at least %s characters long.", param)
	case "max":
		return fmt.Sprintf("Cannot be longer than %s characters.", param)
	case "password":
		return "Must contain an uppercase letter, a lowercase letter, and a digit."
	case "oneof":
		return fmt.Sprintf("Must be one of: %s", param)
	case "uuid":
		return "Must be valid UUID."
	default:
		return "This field is invalid."
	}
}
