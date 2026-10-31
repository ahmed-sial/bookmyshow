package main

import (
	"log/slog"
	"os"

	"github.com/ahmed-sial/bookmyshow/modules/platform/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	slog.Info("initializing database migrations")
	if len(os.Args) < 2 {
		slog.Error("usage: migrate <up | down>")
		os.Exit(1)
	}

	cfg := config.MustLoad()

	slog.Info("running database migrations")
	m, err := migrate.New("file://migrations", cfg.DbUrl)
	if err != nil {
		slog.Error("error while running migration", "error", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "up":
		if err := m.Up(); err != nil {
			slog.Error("migration up failed", "error", err)
			os.Exit(1)
		}
	case "down":
		if err := m.Steps(-1); err != nil {
			slog.Error("migration down failed", "error", err)
			os.Exit(1)
		}
	default:
		slog.Error("unkown command", "error", err)
	}
	slog.Info("database migrations run successful")
}
