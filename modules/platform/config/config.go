package config

import (
	"log/slog"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

type HttpServer struct {
	Addr string `yaml:"address" env-required:"true"`
	Port string `yaml:"port"    env-required:"true"`
}

type AppConfig struct {
	Env        string `yaml:"env" env-required:"true"`
	HttpServer `yaml:"http_server" env-required:"true"`
}

func MustLoad() *AppConfig {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		slog.Error("config path not set")
		os.Exit(1)
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		slog.Error("file doesn't exist on config path", "error", err)
		os.Exit(1)
	}

	var cfg AppConfig

	if err := cleanenv.ReadConfig(cfgPath, &cfg); err != nil {
		slog.Error("failed to read config file", "error", err)
		os.Exit(1)
	}
	return &cfg
}
