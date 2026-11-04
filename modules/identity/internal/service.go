package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ahmed-sial/bookmyshow/modules/platform/config"
	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

type IdentityService struct {
	repo      IdentityRepository
	validator *validator.AppValidator
	config    *config.AppConfig
}

func NewIdentityService(repo IdentityRepository, v *validator.AppValidator, cfg *config.AppConfig) *IdentityService {
	return &IdentityService{repo: repo, validator: v, config: cfg}
}

const dummyHashedPassword = "$2a$10$ZOXOKa/wCzEaxePB7/wyW.iX3XaVn6tP.QpFvfRZxhy7CutXzvBx6"

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

func (s *IdentityService) Login(ctx context.Context, dto UserLoginReqDto) (*UserLoginResDto, error) {
	dto.Email = strings.ToLower(strings.TrimSpace(dto.Email))
	if errs := s.validator.ValidateStruct(dto); len(errs) > 0 {
		return nil, &ValidationError{Fields: errs}
	}
	u, err := s.repo.FindByEmail(ctx, dto.Email)
	if err != nil {
		checkPassword([]byte(dto.Password), []byte(dummyHashedPassword))
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}

	validPassword := checkPassword([]byte(dto.Password), []byte(u.passwordHash))
	if !validPassword {
		return nil, ErrPasswordInvalid
	}

	expirationTime := time.Now().Add(time.Minute * 10)
	claims := &Claims{
		Username:  u.email,
		ExpiresAt: jwt.NewNumericDate(expirationTime),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.config.JwtSecret))
	if err != nil {
		return nil, err
	}

	user := UserLoginResDto{
		ID:          u.id.String(),
		FirstName:   u.firstName,
		LastName:    u.lastName,
		Email:       u.email,
		PhoneNumber: u.phoneNumber,
		Token:       tokenStr,
		ExpiresAt:   expirationTime,
	}
	return &user, nil

}
