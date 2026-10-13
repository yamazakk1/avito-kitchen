package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	defaultHTTPAddress     = ":8081"
	defaultShutdownTimeout = 10 * time.Second
)

type Config struct {
	HTTPAddress     string
	ShutdownTimeout time.Duration
}

func Load(logger *slog.Logger) (Config, error) {
	shutdownTimeout, err := getDurationEnvOrDefault(logger, "DEMO_CAFE_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddress:     getEnvOrDefault(logger, "DEMO_CAFE_HTTP_ADDR", defaultHTTPAddress),
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

func getEnvOrDefault(logger *slog.Logger, name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value != "" {
		return value
	}

	logger.Info("configuration default is used", "variable", name)
	return fallback
}

func getDurationEnvOrDefault(logger *slog.Logger, name string, fallback time.Duration) (time.Duration, error) {
	raw := getEnvOrDefault(logger, name, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return value, nil
}
