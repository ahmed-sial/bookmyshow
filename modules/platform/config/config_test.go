package config

import (
	"os"
	"path/filepath"
	"testing"
)

// MustLoad terminates the process on failure paths, so only the successful
// loads are testable in-process. The failure branches are exercised every day
// by anyone forgetting CONFIG_PATH.
func TestMustLoad(t *testing.T) {
	t.Run("reads the full configuration from CONFIG_PATH", func(t *testing.T) {
		writeConfig(t, `env: test
db_url: postgres://user:password@localhost:5432/bms_db?sslmode=disable
jwt_secret: test-secret
http_server:
  address: 0.0.0.0
  port: "8080"
log:
  level: debug
  format: json
`)

		cfg := MustLoad()

		switch {
		case cfg.Env != "test":
			t.Errorf("env = %q, want test", cfg.Env)
		case cfg.DbUrl != "postgres://user:password@localhost:5432/bms_db?sslmode=disable":
			t.Errorf("db_url = %q", cfg.DbUrl)
		case cfg.JwtSecret != "test-secret":
			t.Errorf("jwt_secret = %q", cfg.JwtSecret)
		case cfg.HttpServer.Addr != "0.0.0.0" || cfg.Port != "8080":
			t.Errorf("http_server = (%q, %q), want (0.0.0.0, 8080)", cfg.HttpServer.Addr, cfg.Port)
		case cfg.Log.Level != "debug" || cfg.Log.Format != "json":
			t.Errorf("log = %+v, want level debug format json", cfg.Log)
		}
	})

	t.Run("log section is optional with sane defaults", func(t *testing.T) {
		writeConfig(t, `env: test
db_url: postgres://user:password@localhost:5432/bms_db?sslmode=disable
jwt_secret: test-secret
http_server:
  address: 0.0.0.0
  port: "8080"
`)

		cfg := MustLoad()

		if cfg.Log.Level != "info" {
			t.Errorf("default log level = %q, want info", cfg.Log.Level)
		}
	})
}

func writeConfig(t *testing.T, yaml string) string {
	t.Helper()

	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgFile, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_PATH", cfgFile)
	return cfgFile
}
