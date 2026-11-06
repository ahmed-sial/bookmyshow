package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ahmed-sial/bookmyshow/modules/platform/config"
	"github.com/ahmed-sial/bookmyshow/modules/platform/types"
	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

// Compile-time check that the fake satisfies the consumer-declared interface.
var _ IdentityRepository = (*fakeIdentityRepository)(nil)

var errRepository = errors.New("repository failure")

type fakeIdentityRepository struct {
	byID    map[uuid.UUID]user
	byEmail map[string]user
	byPhone map[string]user

	createErr      error
	findByIDErr    error
	findByEmailErr error
	deleteErr      error

	createCalls      int
	findByIDCalls    int
	findByEmailCalls int
	deleteCalls      int
}

func newFakeIdentityRepository() *fakeIdentityRepository {
	return &fakeIdentityRepository{
		byID:    make(map[uuid.UUID]user),
		byEmail: make(map[string]user),
		byPhone: make(map[string]user),
	}
}

func (f *fakeIdentityRepository) Create(ctx context.Context, u user) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	// Mimic the partial unique indexes on active rows.
	if _, exists := f.byEmail[u.email]; exists {
		return ErrEmailTaken
	}
	if _, exists := f.byPhone[u.phoneNumber]; exists {
		return ErrPhoneTaken
	}
	f.byID[u.id] = u
	f.byEmail[u.email] = u
	f.byPhone[u.phoneNumber] = u
	return nil
}

func (f *fakeIdentityRepository) FindByID(ctx context.Context, id uuid.UUID) (*user, error) {
	f.findByIDCalls++
	if f.findByIDErr != nil {
		return nil, f.findByIDErr
	}
	if u, ok := f.byID[id]; ok {
		return &u, nil
	}
	return nil, ErrUserNotFound
}

func (f *fakeIdentityRepository) FindByEmail(ctx context.Context, e string) (*user, error) {
	f.findByEmailCalls++
	if f.findByEmailErr != nil {
		return nil, f.findByEmailErr
	}
	if u, ok := f.byEmail[e]; ok {
		return &u, nil
	}
	return nil, ErrUserNotFound
}

func (f *fakeIdentityRepository) DeleteByID(ctx context.Context, id uuid.UUID) error {
	f.deleteCalls++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.byID[id]; !ok {
		return ErrUserNotFound
	}
	u := f.byID[id]
	delete(f.byID, id)
	delete(f.byEmail, u.email)
	delete(f.byPhone, u.phoneNumber)
	return nil
}

func newTestService(repo IdentityRepository) *IdentityService {
	return NewIdentityService(repo, validator.NewAppValidator(), &config.AppConfig{JwtSecret: "unit-test-secret"})
}

func validCreateUserDto() CreateUserDto {
	return CreateUserDto{
		FirstName:   "Ada",
		LastName:    "Lovelace",
		Email:       "Ada@Example.COM",
		Password:    "Sup3rSecret",
		PhoneNumber: "+1 555 0001",
	}
}

// seedUser stores a ready-to-login user, bypassing the DTO normalization so
// that login tests control exactly what the repository returns.
func seedUser(t *testing.T, repo *fakeIdentityRepository, email, password string) user {
	t.Helper()

	hash, err := hashPassword([]byte(password))
	if err != nil {
		t.Fatalf("setup: hashPassword: %v", err)
	}
	u := user{
		id:           uuid.New(),
		firstName:    "Ada",
		lastName:     "Lovelace",
		email:        email,
		passwordHash: string(hash),
		phoneNumber:  "+15550001",
		status:       true,
	}
	repo.byID[u.id] = u
	repo.byEmail[u.email] = u
	repo.byPhone[u.phoneNumber] = u
	return u
}

func TestIdentityService_CreateUser(t *testing.T) {
	t.Parallel()

	password := validCreateUserDto().Password

	t.Run("stores normalized user with hashed password", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		svc := newTestService(repo)

		if err := svc.CreateUser(context.Background(), validCreateUserDto()); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if len(repo.byID) != 1 {
			t.Fatalf("repository holds %d users, want 1", len(repo.byID))
		}

		var stored user
		for _, u := range repo.byID {
			stored = u
		}
		switch {
		case stored.firstName != "Ada":
			t.Errorf("firstName = %q, want %q", stored.firstName, "Ada")
		case stored.email != "ada@example.com":
			t.Errorf("email = %q, want lowercased %q", stored.email, "ada@example.com")
		case stored.phoneNumber != "+15550001":
			t.Errorf("phoneNumber = %q, want spaces removed %q", stored.phoneNumber, "+15550001")
		case stored.passwordHash == password:
			t.Errorf("passwordHash equals the plain password %q", stored.passwordHash)
		case !strings.HasPrefix(stored.passwordHash, "$2"):
			t.Errorf("passwordHash = %q, want a bcrypt hash", stored.passwordHash)
		case !stored.status:
			t.Error("new user stored with status = false")
		case stored.id == uuid.Nil:
			t.Error("new user stored without an id")
		}

		if !checkPassword([]byte(password), []byte(stored.passwordHash)) {
			t.Error("stored hash does not verify the original password")
		}
	})

	failures := []struct {
		name      string
		mutate    func(d *CreateUserDto)
		wantField string
	}{
		{
			name:      "missing first name",
			mutate:    func(d *CreateUserDto) { d.FirstName = "" },
			wantField: "firstName",
		},
		{
			name:      "invalid email",
			mutate:    func(d *CreateUserDto) { d.Email = "not-an-email" },
			wantField: "email",
		},
		{
			name:      "password without digit",
			mutate:    func(d *CreateUserDto) { d.Password = "noDigitsHere" },
			wantField: "password",
		},
		{
			name:      "password without uppercase",
			mutate:    func(d *CreateUserDto) { d.Password = "nodigit0_problem" },
			wantField: "password",
		},
		{
			name:      "password too short",
			mutate:    func(d *CreateUserDto) { d.Password = "aB34567" },
			wantField: "password",
		},
		{
			name:      "invalid phone number without plus",
			mutate:    func(d *CreateUserDto) { d.PhoneNumber = "15550001" },
			wantField: "phoneNumber",
		},
	}

	for _, tt := range failures {
		t.Run("validation failure: "+tt.name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeIdentityRepository()
			svc := newTestService(repo)
			dto := validCreateUserDto()
			tt.mutate(&dto)

			err := svc.CreateUser(context.Background(), dto)
			if err == nil {
				t.Fatal("CreateUser with invalid dto returned no error")
			}

			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v (%T), want *ValidationError", err, err)
			}
			if !fieldReported(ve.Fields, tt.wantField) {
				t.Fatalf("fields = %+v, want a failure on %q", ve.Fields, tt.wantField)
			}
			if repo.createCalls != 0 {
				t.Errorf("repository called %d times for an invalid dto", repo.createCalls)
			}
		})
	}

	t.Run("validation failures never reach the repository (empty dto)", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		svc := newTestService(repo)

		err := svc.CreateUser(context.Background(), CreateUserDto{})
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("err = %v (%T), want *ValidationError", err, err)
		}
		if len(ve.Fields) == 0 {
			t.Error("empty dto produced no field errors")
		}
	})

	t.Run("propagates repository errors", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		repo.createErr = errRepository
		svc := newTestService(repo)

		err := svc.CreateUser(context.Background(), validCreateUserDto())
		if !errors.Is(err, errRepository) {
			t.Fatalf("err = %v, want wrapped errRepository", err)
		}
	})
}

// fieldReported reports whether Fields contains an entry for field.
func fieldReported(errs []validator.FieldError, field string) bool {
	for _, fe := range errs {
		if fe.Field == field {
			return true
		}
	}
	return false
}

func TestIdentityService_FindByID(t *testing.T) {
	t.Parallel()

	t.Run("viewer reading their own record", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		seeded := seedUser(t, repo, "ada@example.com", "Sup3rSecret")
		svc := newTestService(repo)

		dto, err := svc.FindByID(context.Background(), seeded.id, seeded.id)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if dto.ID != seeded.id.String() || dto.FirstName != seeded.firstName ||
			dto.Email != seeded.email || dto.LastName != seeded.lastName ||
			dto.PhoneNumber != seeded.phoneNumber {
			t.Fatalf("dto = %+v, want mapped fields of the stored user", dto)
		}
	})

	t.Run("viewer reading another user is told not found", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		seeded := seedUser(t, repo, "ada@example.com", "Sup3rSecret")
		svc := newTestService(repo)

		_, err := svc.FindByID(context.Background(), seeded.id, uuid.New())
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
		if repo.findByIDCalls != 0 {
			t.Error("repository queried although the viewer was rejected")
		}
	})

	t.Run("propagates repository errors", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		repo.findByIDErr = errRepository
		svc := newTestService(repo)

		id := uuid.New()
		_, err := svc.FindByID(context.Background(), id, id)
		if !errors.Is(err, errRepository) {
			t.Fatalf("err = %v, want wrapped errRepository", err)
		}
	})
}

func TestIdentityService_Login(t *testing.T) {
	t.Parallel()

	const password = "Sup3rSecret"

	t.Run("valid credentials return user and token", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		seeded := seedUser(t, repo, "ada@example.com", password)
		svc := newTestService(repo)

		dto, err := svc.Login(context.Background(), UserLoginReqDto{Email: "ada@example.com", Password: password})
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if dto.ID != seeded.id.String() || dto.Email != seeded.email ||
			dto.FirstName != seeded.firstName || dto.Token == "" {
			t.Fatalf("dto = %+v, want seeded fields and a token", dto)
		}

		claims := &types.Claims{}
		parsed, err := jwt.ParseWithClaims(dto.Token, claims, func(*jwt.Token) (any, error) {
			return []byte("unit-test-secret"), nil
		}, jwt.WithValidMethods([]string{"HS256"}))
		if err != nil || !parsed.Valid {
			t.Fatalf("token does not validate against the configured HS256 secret: err=%v valid=%v", err, parsed != nil && parsed.Valid)
		}
		if claims.Subject != seeded.id.String() {
			t.Errorf("token subject = %q, want %q", claims.Subject, seeded.id)
		}
		if claims.ExpiresAt == nil {
			t.Fatal("token has no expiration")
		}
		want := time.Now().Add(10 * time.Minute)
		delta := want.Sub(claims.ExpiresAt.Time)
		if delta > 5*time.Second || delta < -5*time.Second {
			t.Errorf("token expires after %v, want ~%v", claims.ExpiresAt.Time, want)
		}
	})

	t.Run("login is case-insensitive on email", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		seedUser(t, repo, "ada@example.com", password)
		svc := newTestService(repo)

		_, err := svc.Login(context.Background(), UserLoginReqDto{Email: "  ADA@Example.COM ", Password: password})
		if err != nil {
			t.Fatalf("Login with mixed-case email failed: %v", err)
		}
	})

	t.Run("wrong password rejected", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		seedUser(t, repo, "ada@example.com", password)
		svc := newTestService(repo)

		_, err := svc.Login(context.Background(), UserLoginReqDto{Email: "ada@example.com", Password: "Wr0ngPass!"})
		if !errors.Is(err, ErrPasswordInvalid) {
			t.Fatalf("err = %v, want ErrPasswordInvalid", err)
		}
	})

	t.Run("unknown email rejected without hashing cost ambush", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		svc := newTestService(repo)

		_, err := svc.Login(context.Background(), UserLoginReqDto{Email: "ghost@example.com", Password: password})
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("propagates repository errors", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		repo.findByEmailErr = errRepository
		svc := newTestService(repo)

		_, err := svc.Login(context.Background(), UserLoginReqDto{Email: "ada@example.com", Password: password})
		if !errors.Is(err, errRepository) {
			t.Fatalf("err = %v, want wrapped errRepository", err)
		}
	})

	t.Run("invalid email dto returns validation error without repository call", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		svc := newTestService(repo)

		_, err := svc.Login(context.Background(), UserLoginReqDto{Email: "nope", Password: password})
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("err = %v (%T), want *ValidationError", err, err)
		}
		if repo.findByEmailCalls != 0 {
			t.Error("repository queried although the dto was invalid")
		}
	})
}

func TestIdentityService_DeleteByID(t *testing.T) {
	t.Parallel()

	t.Run("deleting own account succeeds", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		seeded := seedUser(t, repo, "ada@example.com", "Sup3rSecret")
		svc := newTestService(repo)

		if err := svc.DeleteByID(context.Background(), seeded.id, seeded.id); err != nil {
			t.Fatalf("DeleteByID: %v", err)
		}
		if _, err := repo.FindByID(context.Background(), seeded.id); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("user still present after delete: %v", err)
		}
	})

	t.Run("deleting another user is refused as not found", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		seeded := seedUser(t, repo, "ada@example.com", "Sup3rSecret")
		svc := newTestService(repo)

		err := svc.DeleteByID(context.Background(), seeded.id, uuid.New())
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
		if repo.deleteCalls != 0 {
			t.Error("repository reached although the viewer was rejected")
		}
	})

	t.Run("propagates repository errors", func(t *testing.T) {
		t.Parallel()

		repo := newFakeIdentityRepository()
		repo.deleteErr = errRepository
		svc := newTestService(repo)

		id := uuid.New()
		if err := svc.DeleteByID(context.Background(), id, id); !errors.Is(err, errRepository) {
			t.Fatalf("err = %v, want wrapped errRepository", err)
		}
	})
}
