package identity

import (
	"context"

	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

type IdentityService struct {
	repo *IdentityRepository
}

func NewIdentityService(repo *IdentityRepository) *IdentityService {
	return &IdentityService{repo: repo}
}

var appValidator = validator.NewAppValidator()

func (s *IdentityService) CreateUserIdentity(ctx context.Context, u CreateUserDto) {
	if errs := appValidator.ValidateStruct(u); len(errs) > 0 {
	}
	/*
			if errs := appValidator.ValidateStruct(dto); len(errs) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity) // 422 is standard for validation issues

			// Encode your clean, structured 'errs' slice directly to the client
			// e.g., json.NewEncoder(w).Encode(map[string]interface{}{"errors": errs})
			return
		}

		// Data is clean! Proceed to business logic / database
		w.WriteHeader(http.StatusCreated)
	*/
}
