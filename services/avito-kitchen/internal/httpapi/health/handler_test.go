package health

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error {
	return f(ctx)
}

func TestLivenessDoesNotDependOnPostgreSQL(t *testing.T) {
	pingCalls := 0
	h := newTestHandler(pingFunc(func(context.Context) error {
		pingCalls++
		return errors.New("database unavailable")
	}))

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if pingCalls != 0 {
		t.Fatalf("PostgreSQL ping calls = %d, want 0", pingCalls)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", recorder.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(recorder.Body.String()) != `{"status":"ok"}` {
		t.Errorf("body = %q, want liveness response", recorder.Body.String())
	}
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name       string
		ping       pingFunc
		wantStatus int
		wantBody   string
	}{
		{
			name:       "PostgreSQL available",
			ping:       func(context.Context) error { return nil },
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
		},
		{
			name:       "PostgreSQL unavailable",
			ping:       func(context.Context) error { return errors.New("database unavailable") },
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"error":{"code":"DEPENDENCY_UNAVAILABLE","message":"PostgreSQL is unavailable"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.ping)
			recorder := httptest.NewRecorder()

			h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if strings.TrimSpace(recorder.Body.String()) != tt.wantBody {
				t.Errorf("body = %q, want %q", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestReadinessHonorsTimeout(t *testing.T) {
	h := newHandler(
		pingFunc(func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}),
		10*time.Millisecond,
	)
	recorder := httptest.NewRecorder()

	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func newTestHandler(pinger DatabasePinger) http.Handler {
	return newHandler(pinger, time.Second)
}

func newHandler(pinger DatabasePinger, timeout time.Duration) http.Handler {
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), pinger, timeout)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}
