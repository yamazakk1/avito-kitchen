package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	cafeclient "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/client/cafe"
	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/config"
	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/httpapi"
	menurepository "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/repository/postgres/menu"
	orderrepository "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/repository/postgres/order"
	restaurantrepository "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/repository/postgres/restaurant"
	menuservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/menu"
	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
	restaurantservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/restaurant"
	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/worker/orderprocessor"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

type orderProcessor interface {
	Run(context.Context)
}

type App struct {
	logger         *slog.Logger
	config         config.Config
	pool           *pgxpool.Pool
	server         *http.Server
	orderProcessor orderProcessor
	buildErr       error
}

func BuildAppFromConfig() *App {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	application := &App{
		logger: logger,
	}
	cfg, err := config.Load(logger)
	if err != nil {
		application.buildErr = fmt.Errorf("load configuration: %w", err)
		return application
	}
	application.config = cfg

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		application.buildErr = fmt.Errorf("parse PostgreSQL configuration: %w", err)
		return application
	}
	poolConfig.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		application.buildErr = fmt.Errorf("create PostgreSQL connection pool: %w", err)
		return application
	}

	application.pool = pool
	restaurantRepository := restaurantrepository.NewRepository(pool)
	restaurantService := restaurantservice.NewService(restaurantRepository)
	menuRepository := menurepository.NewRepository(pool)
	menuService := menuservice.NewService(menuRepository)
	orderRepository := orderrepository.NewRepository(pool)
	orderService := orderservice.NewService(orderRepository)
	cafeClient := cafeclient.NewClient(cfg.CafeRequestTimeout)
	application.orderProcessor = orderprocessor.NewProcessor(
		logger,
		orderRepository,
		cafeClient,
		cfg.OrderProcessorInterval,
	)
	application.server = &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           httpapi.NewHandler(logger, pool, restaurantService, menuService, orderService, cfg.ReadinessTimeout),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	return application
}

func (a *App) Run() int {
	if a.buildErr != nil {
		a.logger.Error("build application", "error", a.buildErr)
		return 1
	}

	if err := a.run(); err != nil {
		a.logger.Error("application stopped with an error", "error", err)
		return 1
	}

	return 0
}

func (a *App) run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer a.pool.Close()

	pingCtx, cancelPing := context.WithTimeout(ctx, a.config.DatabasePingTimeout)
	err := a.pool.Ping(pingCtx)
	cancelPing()
	if err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	a.logger.Info("PostgreSQL is available")

	listener, err := net.Listen("tcp", a.config.HTTPAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.config.HTTPAddress, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- a.server.Serve(listener)
	}()

	processorDone := make(chan struct{})
	go func() {
		defer close(processorDone)
		a.orderProcessor.Run(ctx)
	}()

	a.logger.Info("HTTP server started", "address", listener.Addr().String())
	a.logger.Info("order processor started", "poll_interval", a.config.OrderProcessorInterval)

	serverStopped := false
	var serverFailure error

	select {
	case err = <-serveErr:
		serverStopped = true
		if !errors.Is(err, http.ErrServerClosed) {
			serverFailure = fmt.Errorf("serve HTTP: %w", err)
		}
		stop()
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), a.config.ShutdownTimeout)
	defer cancelShutdown()

	var shutdownFailure error
	if !serverStopped {
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			shutdownFailure = fmt.Errorf("shutdown HTTP server: %w", err)
		} else if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverFailure = fmt.Errorf("serve HTTP during shutdown: %w", err)
		}
	}

	stop()
	select {
	case <-processorDone:
	case <-shutdownCtx.Done():
		return fmt.Errorf("shutdown order processor: %w", shutdownCtx.Err())
	}

	if shutdownFailure != nil {
		return shutdownFailure
	}
	if serverFailure != nil {
		return serverFailure
	}

	a.logger.Info("application stopped gracefully")
	return nil
}
