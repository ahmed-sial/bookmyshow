//go:build integration

package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// These tests run against a real Postgres, never the dev database. Point
// BMS_TEST_DB_URL at a scratch database and run:
//
//	go test -race -count=1 -tags=integration ./modules/identity/internal
//
// A test database should be thrown away between runs; the users table is
// truncated before every case (:active-only uniqueness is verified here too).

var userSeq atomic.Int64

// testUser builds a schema-valid user with a unique email and phone so the
// unique indexes never fire by accident.
func testUser() user {
	seq := userSeq.Add(1)
	return user{
		id:           uuid.New(),
		firstName:    "Ada",
		lastName:     "Lovelace",
		email:        fmt.Sprintf("ada-%d@integration.example.com", seq),
		passwordHash: "test-hash-does-not-need-to-be-bcrypt",
		phoneNumber:  fmt.Sprintf("+1%010d", seq),
		status:       true,
	}
}

// migrationsDir resolves the repository migrations folder on disk regardless
// of the working directory go test happens to run in.
func migrationsDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller could not resolve the repo root")
	}
	// file = .../migrations/identity/internal/repository_integration_test.go is
	// three levels above the repo root: internal -> identity -> modules -> root.
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
	return filepath.Join(root, "migrations")
}

func newIntegrationRepository(t *testing.T) (*PostgresIdentityRepository, *sql.DB) {
	t.Helper()

	url := os.Getenv("BMS_TEST_DB_URL")
	if url == "" {
		t.Skip("BMS_TEST_DB_URL not set; skipping repository integration tests")
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	waitForDB(t, db)

	m, err := migrate.New("file://"+migrationsDir(), url)
	if err != nil {
		t.Fatalf("build migration runner: %v", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("apply migrations: %v", err)
	}

	truncateUsers(t, db)
	return NewPostgresIdentityRepository(db), db
}

// waitForDB gives a freshly started Postgres time to accept connections. A
// container that is up but not ready must not look like a broken test build.
func waitForDB(t *testing.T, db *sql.DB) {
	t.Helper()

	const timeout = 20 * time.Second
	deadline := time.Now().Add(timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		err := db.PingContext(ctx)
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("test database not ready within %v: %v", timeout, err)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func truncateUsers(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec("TRUNCATE users"); err != nil {
		t.Fatalf("truncate users: %v", err)
	}
}

func assertUserNotFound(t *testing.T, err error) {
	t.Helper()

	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v (%T), want ErrUserNotFound", err, err)
	}
}

func TestRepositoryCreate(t *testing.T) {
	repo, db := newIntegrationRepository(t)
	ctx := context.Background()

	t.Run("created user is found with intact fields", func(t *testing.T) {
		truncateUsers(t, db)
		u := testUser()

		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.FindByID(ctx, u.id)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		switch {
		case got.id != u.id:
			t.Errorf("id = %v, want %v", got.id, u.id)
		case got.firstName != u.firstName:
			t.Errorf("firstName = %q, want %q", got.firstName, u.firstName)
		case got.lastName != u.lastName:
			t.Errorf("lastName = %q, want %q", got.lastName, u.lastName)
		case got.email != u.email:
			t.Errorf("email = %q, want %q", got.email, u.email)
		case got.phoneNumber != u.phoneNumber:
			t.Errorf("phoneNumber = %q, want %q", got.phoneNumber, u.phoneNumber)
		}
	})

	t.Run("empty last name round-trips through NULL", func(t *testing.T) {
		truncateUsers(t, db)
		u := testUser()
		u.lastName = ""

		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.FindByID(ctx, u.id)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.lastName != "" {
			t.Errorf("lastName = %q, want empty (stored as NULL)", got.lastName)
		}
	})

	t.Run("duplicate active email is ErrEmailTaken", func(t *testing.T) {
		truncateUsers(t, db)
		first := testUser()
		if err := repo.Create(ctx, first); err != nil {
			t.Fatalf("setup: Create: %v", err)
		}

		duplicate := testUser()
		duplicate.email = first.email

		err := repo.Create(ctx, duplicate)
		if !errors.Is(err, ErrEmailTaken) {
			t.Fatalf("err = %v (%T), want ErrEmailTaken", err, err)
		}
	})

	t.Run("duplicate active phone is ErrPhoneTaken", func(t *testing.T) {
		truncateUsers(t, db)
		first := testUser()
		if err := repo.Create(ctx, first); err != nil {
			t.Fatalf("setup: Create: %v", err)
		}

		duplicate := testUser()
		duplicate.phoneNumber = first.phoneNumber

		err := repo.Create(ctx, duplicate)
		if !errors.Is(err, ErrPhoneTaken) {
			t.Fatalf("err = %v (%T), want ErrPhoneTaken", err, err)
		}
	})

	t.Run("soft-deleted rows release email and phone", func(t *testing.T) {
		truncateUsers(t, db)
		original := testUser()
		if err := repo.Create(ctx, original); err != nil {
			t.Fatalf("setup: Create: %v", err)
		}
		if err := repo.DeleteByID(ctx, original.id); err != nil {
			t.Fatalf("setup: DeleteByID: %v", err)
		}

		// The soft-deleted row still exists physically; a fresh user must be
		// able to claim the same email and phone again.
		reuse := testUser()
		reuse.email = original.email
		reuse.phoneNumber = original.phoneNumber
		if err := repo.Create(ctx, reuse); err != nil {
			t.Fatalf("Create reusing deleted identity: %v", err)
		}
	})
}

func TestRepositoryFind(t *testing.T) {
	repo, db := newIntegrationRepository(t)
	ctx := context.Background()

	t.Run("unknown id is ErrUserNotFound", func(t *testing.T) {
		_, err := repo.FindByID(ctx, uuid.New())
		assertUserNotFound(t, err)
	})

	t.Run("unknown email is ErrUserNotFound", func(t *testing.T) {
		_, err := repo.FindByEmail(ctx, "nobody@integration.example.com")
		assertUserNotFound(t, err)
	})

	t.Run("FindByEmail returns the password hash for login", func(t *testing.T) {
		truncateUsers(t, db)
		u := testUser()

		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.FindByEmail(ctx, u.email)
		if err != nil {
			t.Fatalf("FindByEmail: %v", err)
		}
		switch {
		case got.id != u.id:
			t.Errorf("id = %v, want %v", got.id, u.id)
		case got.passwordHash != u.passwordHash:
			t.Errorf("passwordHash = %q, want %q (login needs this column)", got.passwordHash, u.passwordHash)
		}
	})
}

func TestRepositoryDelete(t *testing.T) {
	repo, db := newIntegrationRepository(t)
	ctx := context.Background()

	t.Run("deleted user disappears from both lookups", func(t *testing.T) {
		truncateUsers(t, db)
		u := testUser()
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("setup: Create: %v", err)
		}

		if err := repo.DeleteByID(ctx, u.id); err != nil {
			t.Fatalf("DeleteByID: %v", err)
		}
		_, err := repo.FindByID(ctx, u.id)
		assertUserNotFound(t, err)
		_, err = repo.FindByEmail(ctx, u.email)
		assertUserNotFound(t, err)
	})

	t.Run("second delete is ErrUserNotFound", func(t *testing.T) {
		truncateUsers(t, db)
		u := testUser()
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("setup: Create: %v", err)
		}
		if err := repo.DeleteByID(ctx, u.id); err != nil {
			t.Fatalf("setup: DeleteByID: %v", err)
		}

		err := repo.DeleteByID(ctx, u.id)
		assertUserNotFound(t, err)
	})

	t.Run("deleting an unknown id is ErrUserNotFound", func(t *testing.T) {
		err := repo.DeleteByID(ctx, uuid.New())
		assertUserNotFound(t, err)
	})
}
