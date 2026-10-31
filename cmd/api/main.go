package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ahmed-sial/bookmyshow/modules/platform/config"
)

func main() {
	slog.Info("loading configuration")
	cfg := config.MustLoad()
	slog.Info("configuration loaded successfully")

	mux := http.NewServeMux()
	server := http.Server{
		Addr:         net.JoinHostPort(cfg.Addr, cfg.Port),
		Handler:      mux,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 30,
		IdleTimeout:  time.Second * 60,
	}

	closeChan := make(chan os.Signal, 1)
	signal.Notify(closeChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("server running on", "address", server.Addr)
	go func() {
		if err := server.ListenAndServe(); err != nil {
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
