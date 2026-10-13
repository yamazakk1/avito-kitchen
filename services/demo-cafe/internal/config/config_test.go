package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		shutdown    string
		wantAddress string
		wantTimeout time.Duration
		wantError   bool
	}{
		{
			name:        "defaults",
			wantAddress: defaultHTTPAddress,
			wantTimeout: 10 * time.Second,
		},
		{
			name:        "custom values",
			address:     ":9091",
			shutdown:    "15s",
			wantAddress: ":9091",
			wantTimeout: 15 * time.Second,
		},
		{
			name:      "invalid shutdown timeout",
			shutdown:  "later",
			wantError: true,
		},
		{
			name:      "non-positive shutdown timeout",
			shutdown:  "-1s",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEMO_CAFE_HTTP_ADDR", tt.address)
			t.Setenv("DEMO_CAFE_SHUTDOWN_TIMEOUT", tt.shutdown)

			cfg, err := Load(slog.Default())
			if (err != nil) != tt.wantError {
				t.Fatalf("Load() error = %v, wantError %t", err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if cfg.HTTPAddress != tt.wantAddress {
				t.Errorf("HTTPAddress = %q, want %q", cfg.HTTPAddress, tt.wantAddress)
			}
			if cfg.ShutdownTimeout != tt.wantTimeout {
				t.Errorf("ShutdownTimeout = %s, want %s", cfg.ShutdownTimeout, tt.wantTimeout)
			}
		})
	}
}

func TestLoadLogsDefaultUsage(t *testing.T) {
	t.Setenv("DEMO_CAFE_HTTP_ADDR", "")
	t.Setenv("DEMO_CAFE_SHUTDOWN_TIMEOUT", "")

	var logs bytes.Buffer
	if _, err := Load(slog.New(slog.NewJSONHandler(&logs, nil))); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !strings.Contains(logs.String(), "DEMO_CAFE_HTTP_ADDR") {
		t.Errorf("default usage was not logged: %s", logs.String())
	}
}
