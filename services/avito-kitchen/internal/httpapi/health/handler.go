package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/common"
)

type DatabasePinger interface {
	Ping(context.Context) error
}

type Handler struct {
	logger           *slog.Logger
	database         DatabasePinger
	readinessTimeout time.Duration
}

func NewHandler(logger *slog.Logger, database DatabasePinger, readinessTimeout time.Duration) *Handler {
	return &Handler{
		logger:           logger,
		database:         database,
		readinessTimeout: readinessTimeout,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health/live", h.live)
	mux.HandleFunc("GET /health/ready", h.ready)
}

func (h *Handler) live(w http.ResponseWriter, r *http.Request) {
	common.WriteJSON(r.Context(), h.logger, w, http.StatusOK, response{Status: "ok"})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.readinessTimeout)
	defer cancel()

	if err := h.database.Ping(ctx); err != nil {
		h.logger.WarnContext(r.Context(), "readiness check failed", "dependency", "postgresql", "error", err)
		common.WriteError(
			r.Context(),
			h.logger,
			w,
			http.StatusServiceUnavailable,
			"DEPENDENCY_UNAVAILABLE",
			"PostgreSQL is unavailable",
		)
		return
	}

	common.WriteJSON(r.Context(), h.logger, w, http.StatusOK, response{Status: "ok"})
}
