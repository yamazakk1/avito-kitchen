package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	orderservice "github.com/yamazakk1/avito-kitchen/services/demo-cafe/internal/service/order"
)

type stubOrderService struct {
	result orderservice.Result
	err    error
}

func (s stubOrderService) SubmitOrder(context.Context, orderservice.SubmitOrderInput) (orderservice.Result, error) {
	return s.result, s.err
}

func TestLiveness(t *testing.T) {
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), stubOrderService{})
	recorder := httptest.NewRecorder()

	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", recorder.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(recorder.Body.String()) != `{"status":"ok"}` {
		t.Errorf("body = %q, want liveness response", recorder.Body.String())
	}
}
