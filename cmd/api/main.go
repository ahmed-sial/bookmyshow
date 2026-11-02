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
	"github.com/ahmed-sial/bookmyshow/modules/platform/validator"
)

func main() {
	slog.Info("loading configuration")
	cfg := config.MustLoad()
	slog.Info("configuration loaded successfully")

	slog.Info("connecting to db")
	db, err := database.Connect(cfg.DbUrl)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	slog.Info("db connection successful")

	defer db.Close()
	v := validator.NewAppValidator()
	mux := http.NewServeMux()
	identity.RegisterRoutes(mux, db, v)
	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.Addr, cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: time.Second * 5,
		ReadTimeout:       time.Second * 10,
		WriteTimeout:      time.Second * 30,
		IdleTimeout:       time.Second * 60,
	}

	closeChan := make(chan os.Signal, 1)
	signal.Notify(closeChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("server running on", "address", server.Addr)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failure", "error", err.Error())
			os.Exit(1)
		}
	}()

	<-closeChan
	slog.Info("terminating server...")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("failed to terminate server", "error", err.Error())
	}
	slog.Info("server terminated successfully")

}
