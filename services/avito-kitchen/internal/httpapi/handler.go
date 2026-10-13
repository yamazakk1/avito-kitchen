package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	healthapi "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/health"
	menuapi "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/menu"
	orderapi "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/order"
	restaurantapi "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi/restaurant"
)

func NewHandler(
	logger *slog.Logger,
	database healthapi.DatabasePinger,
	restaurantService restaurantapi.Service,
	menuService menuapi.Service,
	orderService orderapi.Service,
	readinessTimeout time.Duration,
) http.Handler {
	mux := http.NewServeMux()
	healthapi.NewHandler(logger, database, readinessTimeout).RegisterRoutes(mux)
	restaurantapi.NewHandler(logger, restaurantService).RegisterRoutes(mux)
	menuapi.NewHandler(logger, menuService).RegisterRoutes(mux)
	orderapi.NewHandler(logger, orderService).RegisterRoutes(mux)
	return mux
}
