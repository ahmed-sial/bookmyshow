package validator

import (
	"testing"
)

func TestValidateStruct(t *testing.T) {
	t.Parallel()

	v := NewAppValidator()

	type payload struct {
		Email    string `json:"email"      validate:"required,email,max=255"`
		Password string `json:"password"   validate:"required,min=8,max=72,password"`
		Phone    string `json:"phoneNumber" validate:"required,phone"`
	}

	base := payload{
		Email:    "ada@example.com",
		Password: "Sup3rSecret",
		Phone:    "+15550001",
	}

	t.Run("valid payload produces no errors", func(t *testing.T) {
		t.Parallel()

		if errs := v.ValidateStruct(base); errs != nil {
			t.Fatalf("errs = %+v, want none", errs)
		}
	})

	failures := []struct {
		name        string
		mutate      func(p *payload)
		wantField   string
		wantMessage string
	}{
		{
			name:        "missing email",
			mutate:      func(p *payload) { p.Email = "" },
			wantField:   "email",
			wantMessage: "This field is required.",
		},
		{
			name:        "malformed email",
			mutate:      func(p *payload) { p.Email = "not-an-email" },
			wantField:   "email",
			wantMessage: "This must be a valid email address.",
		},
		{
			name:        "password without uppercase",
			mutate:      func(p *payload) { p.Password = "nodigit0_pass" },
			wantField:   "password",
			wantMessage: "Must contain an uppercase letter, a lowercase letter, and a digit.",
		},
		{
			name:        "password without lowercase",
			mutate:      func(p *payload) { p.Password = "NODIGIT0_PASS" },
			wantField:   "password",
			wantMessage: "Must contain an uppercase letter, a lowercase letter, and a digit.",
		},
		{
			name:        "password without digit",
			mutate:      func(p *payload) { p.Password = "NoDigitsHere" },
			wantField:   "password",
			wantMessage: "Must contain an uppercase letter, a lowercase letter, and a digit.",
		},
		{
			name:        "password too short",
			mutate:      func(p *payload) { p.Password = "aB34567" },
			wantField:   "password",
			wantMessage: "Must be at least 8 characters long.",
		},
		{
			name:        "password too long",
			mutate:      func(p *payload) { p.Password = repeat("aB3", 30) },
			wantField:   "password",
			wantMessage: "Cannot be longer than 72 characters.",
		},
		{
			name:        "phone without leading plus",
			mutate:      func(p *payload) { p.Phone = "15550001" },
			wantField:   "phoneNumber",
			wantMessage: "Must be a valid international phone number format (e.g., +1234567890).",
		},
		{
			name:        "phone with plus but too few digits",
			mutate:      func(p *payload) { p.Phone = "+12" },
			wantField:   "phoneNumber",
			wantMessage: "Must be a valid international phone number format (e.g., +1234567890).",
		},
	}

	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := base
			tt.mutate(&p)

			errs := v.ValidateStruct(p)
			if len(errs) == 0 {
				t.Fatal("expected validation errors, got none")
			}

			var match *FieldError
			for i := range errs {
				if errs[i].Field == tt.wantField {
					match = &errs[i]
					break
				}
			}
			if match == nil {
				t.Fatalf("errors = %+v, want one on field %q", errs, tt.wantField)
			}
			if match.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", match.Message, tt.wantMessage)
			}
		})
	}

	t.Run("reports json tag names, not Go field names", func(t *testing.T) {
		t.Parallel()

		p := base
		p.Password = "noDigitsHere"

		for _, fe := range v.ValidateStruct(p) {
			if fe.Field == "Password" {
				t.Error("go-playground returned the Go field name; RegisterTagNameFunc is not applied")
			}
		}
	})

	t.Run("nil payload falls back to a system error", func(t *testing.T) {
		t.Parallel()

		errs := v.ValidateStruct(nil)
		if len(errs) != 1 {
			t.Fatalf("errs = %+v, want exactly one fallback error", errs)
		}
		if errs[0].Field != "system" {
			t.Errorf("field = %q, want system", errs[0].Field)
		}
	})
}

func TestValidateStructOtherMessages(t *testing.T) {
	t.Parallel()

	v := NewAppValidator()

	type tiered struct {
		Tier  string `json:"tier"   validate:"required,oneof=small medium large"`
		Actor string `json:"actorId" validate:"omitempty,uuid"`
	}

	tests := []struct {
		name        string
		payload     tiered
		wantField   string
		wantMessage string
	}{
		{
			name:        "oneof message lists options",
			payload:     tiered{Tier: "huge"},
			wantField:   "tier",
			wantMessage: "Must be one of: small medium large",
		},
		{
			name:        "uuid message",
			payload:     tiered{Tier: "small", Actor: "nope"},
			wantField:   "actorId",
			wantMessage: "Must be valid UUID.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			errs := v.ValidateStruct(tt.payload)
			if len(errs) == 0 {
				t.Fatal("expected a validation error, got none")
			}
			for _, fe := range errs {
				if fe.Field == tt.wantField {
					if fe.Message != tt.wantMessage {
						t.Errorf("message = %q, want %q", fe.Message, tt.wantMessage)
					}
					return
				}
			}
			t.Fatalf("errors = %+v, want one on field %q", errs, tt.wantField)
		})
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}
