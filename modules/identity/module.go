package identity

import (
	"database/sql"
	"log/slog"
	"net/http"

	internal "github.com/ahmed-sial/bookmyshow/modules/identity/internal"
	"github.com/ahmed-sial/bookmyshow/modules/platform/config"
	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

func RegisterRoutes(mux *http.ServeMux, db *sql.DB, v *validator.AppValidator, cfg *config.AppConfig, log *slog.Logger, auth func(http.Handler) http.Handler) {
	repo := internal.NewPostgresIdentityRepository(db)
	svc := internal.NewIdentityService(repo, v, cfg)
	h := internal.NewHandler(svc, log)
	internal.RegisterRoutes(mux, h, auth)
}
