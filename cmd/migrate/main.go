package main

import (
	"log/slog"
	"os"

	"github.com/ahmed-sial/bookmyshow/modules/platform/config"
	"github.com/ahmed-sial/bookmyshow/modules/platform/logger"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	cfg := config.MustLoad()

	log := logger.MustNew(logger.Config{
		Level:  cfg.Log.Level,
		Format: cfg.Log.Format,
		Env:    cfg.Env,
	})
	slog.SetDefault(log)

	log.Info("initializing database migrations")
	if len(os.Args) < 2 {
		log.Error("usage: migrate <up | down>")
		os.Exit(1)
	}

	m, err := migrate.New("file://migrations", cfg.DbUrl)
	if err != nil {
		log.Error("error while running migration", "error", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "up":
		if err := m.Up(); err != nil {
			if err == migrate.ErrNoChange {
				break
			} else {
				log.Error("migration up failed", "error", err)
				os.Exit(1)
			}
		}
	case "down":
		if err := m.Steps(-1); err != nil {
			log.Error("migration down failed", "error", err)
			os.Exit(1)
		}
	default:
		log.Error("unknown command", "command", os.Args[1])
		os.Exit(1)
	}
	log.Info("database migrations run successful")
}
