package identity

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

type IdentityService struct {
	repo      IdentityRepository
	validator *validator.AppValidator
}

func NewIdentityService(repo IdentityRepository, v *validator.AppValidator) *IdentityService {
	return &IdentityService{repo: repo, validator: v}
}

func (s *IdentityService) CreateUser(ctx context.Context, dto CreateUserDto) error {
	dto.FirstName = strings.TrimSpace(dto.FirstName)
	dto.LastName = strings.TrimSpace(dto.LastName)
	dto.Email = strings.ToLower(strings.TrimSpace(dto.Email))
	dto.PhoneNumber = strings.ReplaceAll(strings.TrimSpace(dto.PhoneNumber), " ", "")

	if errs := s.validator.ValidateStruct(dto); len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	hash, err := hashPassword([]byte(dto.Password))
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	u := user{
		id:           uuid.New(),
		firstName:    dto.FirstName,
		lastName:     dto.LastName,
		email:        dto.Email,
		passwordHash: string(hash),
		phoneNumber:  dto.PhoneNumber,
		status:       true,
	}
	return s.repo.Create(ctx, u)
}

func (s *IdentityService) FindByID(ctx context.Context, id uuid.UUID) (*GetUserDto, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	userDto := GetUserDto{
		ID:          u.id.String(),
		FirstName:   u.firstName,
		LastName:    u.lastName,
		Email:       u.email,
		PhoneNumber: u.phoneNumber,
	}
	return &userDto, nil
}
