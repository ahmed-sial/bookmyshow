package identity

import (
	"database/sql"
	"log/slog"
	"net/http"

	internal "github.com/ahmed-sial/bookmyshow/modules/identity/internal"
	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

func RegisterRoutes(mux *http.ServeMux, db *sql.DB, v *validator.AppValidator, log *slog.Logger) {
	repo := internal.NewPostgresIdentityRepository(db)
	svc := internal.NewIdentityService(repo, v)
	h := internal.NewHandler(svc, log)
	internal.RegisterRoutes(mux, h)
}
