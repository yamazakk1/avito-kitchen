package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	keys := []string{
		"AVITO_KITCHEN_DATABASE_URL",
		"AVITO_KITCHEN_HTTP_ADDR",
		"AVITO_KITCHEN_DATABASE_CONNECT_TIMEOUT",
		"AVITO_KITCHEN_DATABASE_PING_TIMEOUT",
		"AVITO_KITCHEN_READINESS_TIMEOUT",
		"AVITO_KITCHEN_SHUTDOWN_TIMEOUT",
		"AVITO_KITCHEN_ORDER_PROCESSOR_INTERVAL",
		"AVITO_KITCHEN_CAFE_REQUEST_TIMEOUT",
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}

	var logs bytes.Buffer
	cfg, err := Load(slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddress != defaultHTTPAddress {
		t.Errorf("HTTPAddress = %q, want %q", cfg.HTTPAddress, defaultHTTPAddress)
	}
	if cfg.DatabaseURL != defaultDatabaseURL {
		t.Errorf("DatabaseURL = %q, want default", cfg.DatabaseURL)
	}
	if cfg.DatabaseConnectTimeout != 5*time.Second {
		t.Errorf("DatabaseConnectTimeout = %s, want 5s", cfg.DatabaseConnectTimeout)
	}
	if cfg.DatabasePingTimeout != 3*time.Second {
		t.Errorf("DatabasePingTimeout = %s, want 3s", cfg.DatabasePingTimeout)
	}
	if cfg.ReadinessTimeout != 2*time.Second {
		t.Errorf("ReadinessTimeout = %s, want 2s", cfg.ReadinessTimeout)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 10s", cfg.ShutdownTimeout)
	}
	if cfg.OrderProcessorInterval != time.Second {
		t.Errorf("OrderProcessorInterval = %s, want 1s", cfg.OrderProcessorInterval)
	}
	if cfg.CafeRequestTimeout != 3*time.Second {
		t.Errorf("CafeRequestTimeout = %s, want 3s", cfg.CafeRequestTimeout)
	}
	if !strings.Contains(logs.String(), "AVITO_KITCHEN_HTTP_ADDR") {
		t.Errorf("default usage was not logged: %s", logs.String())
	}
}

func TestLoadCustomValues(t *testing.T) {
	t.Setenv("AVITO_KITCHEN_DATABASE_URL", "postgres://user:password@postgres:5432/database")
	t.Setenv("AVITO_KITCHEN_HTTP_ADDR", ":9090")
	t.Setenv("AVITO_KITCHEN_DATABASE_CONNECT_TIMEOUT", "7s")
	t.Setenv("AVITO_KITCHEN_DATABASE_PING_TIMEOUT", "4s")
	t.Setenv("AVITO_KITCHEN_READINESS_TIMEOUT", "1500ms")
	t.Setenv("AVITO_KITCHEN_SHUTDOWN_TIMEOUT", "12s")
	t.Setenv("AVITO_KITCHEN_ORDER_PROCESSOR_INTERVAL", "250ms")
	t.Setenv("AVITO_KITCHEN_CAFE_REQUEST_TIMEOUT", "2s")

	cfg, err := Load(slog.Default())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddress != ":9090" {
		t.Errorf("HTTPAddress = %q, want %q", cfg.HTTPAddress, ":9090")
	}
	if cfg.DatabaseConnectTimeout != 7*time.Second {
		t.Errorf("DatabaseConnectTimeout = %s, want 7s", cfg.DatabaseConnectTimeout)
	}
	if cfg.DatabasePingTimeout != 4*time.Second {
		t.Errorf("DatabasePingTimeout = %s, want 4s", cfg.DatabasePingTimeout)
	}
	if cfg.ReadinessTimeout != 1500*time.Millisecond {
		t.Errorf("ReadinessTimeout = %s, want 1.5s", cfg.ReadinessTimeout)
	}
	if cfg.ShutdownTimeout != 12*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 12s", cfg.ShutdownTimeout)
	}
	if cfg.OrderProcessorInterval != 250*time.Millisecond {
		t.Errorf("OrderProcessorInterval = %s, want 250ms", cfg.OrderProcessorInterval)
	}
	if cfg.CafeRequestTimeout != 2*time.Second {
		t.Errorf("CafeRequestTimeout = %s, want 2s", cfg.CafeRequestTimeout)
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("AVITO_KITCHEN_DATABASE_CONNECT_TIMEOUT", "")
	t.Setenv("AVITO_KITCHEN_DATABASE_PING_TIMEOUT", "soon")
	t.Setenv("AVITO_KITCHEN_READINESS_TIMEOUT", "")
	t.Setenv("AVITO_KITCHEN_SHUTDOWN_TIMEOUT", "")
	t.Setenv("AVITO_KITCHEN_ORDER_PROCESSOR_INTERVAL", "0s")
	t.Setenv("AVITO_KITCHEN_CAFE_REQUEST_TIMEOUT", "")

	if _, err := Load(slog.Default()); err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
}
