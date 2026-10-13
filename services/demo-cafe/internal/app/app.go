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

	"github.com/yamazakk1/avito-kitchen/services/demo-cafe/internal/config"
	"github.com/yamazakk1/avito-kitchen/services/demo-cafe/internal/httpapi"
	orderservice "github.com/yamazakk1/avito-kitchen/services/demo-cafe/internal/service/order"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

type App struct {
	logger   *slog.Logger
	config   config.Config
	server   *http.Server
	buildErr error
}

func BuildAppFromConfig() *App {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(logger)
	if err != nil {
		return &App{
			logger:   logger,
			buildErr: fmt.Errorf("load configuration: %w", err),
		}
	}

	return &App{
		logger: logger,
		config: cfg,
		server: &http.Server{
			Addr:              cfg.HTTPAddress,
			Handler:           httpapi.NewHandler(logger, orderservice.NewService(orderservice.DefaultState())),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
	}
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

	listener, err := net.Listen("tcp", a.config.HTTPAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.config.HTTPAddress, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- a.server.Serve(listener)
	}()

	a.logger.Info("HTTP server started", "address", listener.Addr().String())

	select {
	case err = <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), a.config.ShutdownTimeout)
	defer cancelShutdown()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP during shutdown: %w", err)
	}

	a.logger.Info("application stopped gracefully")
	return nil
}
