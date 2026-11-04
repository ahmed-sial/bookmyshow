package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ahmed-sial/bookmyshow/modules/identity"
	"github.com/ahmed-sial/bookmyshow/modules/platform/config"
	"github.com/ahmed-sial/bookmyshow/modules/platform/database"
	"github.com/ahmed-sial/bookmyshow/modules/platform/logger"
	"github.com/ahmed-sial/bookmyshow/modules/platform/middleware"
	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

func main() {
	cfg := config.MustLoad()

	log := logger.MustNew(logger.Config{
		Level:  cfg.Log.Level,
		Format: cfg.Log.Format,
		Env:    cfg.Env,
	})
	slog.SetDefault(log)

	log.Info("connecting to db", "env", cfg.Env)
	db, err := database.Connect(cfg.DbUrl)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	log.Info("db connection successful")

	defer db.Close()

	v := validator.NewAppValidator()
	mux := http.NewServeMux()
	identity.RegisterRoutes(mux, db, v, cfg, log)

	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.Addr, cfg.Port),
		Handler:           middleware.RequestLogger(log)(mux),
		ReadHeaderTimeout: time.Second * 5,
		ReadTimeout:       time.Second * 10,
		WriteTimeout:      time.Second * 30,
		IdleTimeout:       time.Second * 60,
	}

	closeChan := make(chan os.Signal, 1)
	signal.Notify(closeChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Info("server running", "address", server.Addr)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failure", "error", err)
			os.Exit(1)
		}
	}()

	<-closeChan
	log.Info("terminating server")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Error("failed to terminate server", "error", err)
	}
	log.Info("server terminated successfully")
}
