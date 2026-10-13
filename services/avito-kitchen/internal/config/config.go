package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	defaultHTTPAddress            = ":8080"
	defaultDatabaseURL            = "postgres://avito:avito@127.0.0.1:5433/avito_kitchen?sslmode=disable"
	defaultDatabaseConnectTimeout = 5 * time.Second
	defaultDatabasePingTimeout    = 3 * time.Second
	defaultReadinessTimeout       = 2 * time.Second
	defaultShutdownTimeout        = 10 * time.Second
	defaultOrderProcessorInterval = 1 * time.Second
	defaultCafeRequestTimeout     = 3 * time.Second
)

type Config struct {
	HTTPAddress            string
	DatabaseURL            string
	DatabaseConnectTimeout time.Duration
	DatabasePingTimeout    time.Duration
	ReadinessTimeout       time.Duration
	ShutdownTimeout        time.Duration
	OrderProcessorInterval time.Duration
	CafeRequestTimeout     time.Duration
}

func Load(logger *slog.Logger) (Config, error) {
	databaseConnectTimeout, err := getDurationEnvOrDefault(logger, "AVITO_KITCHEN_DATABASE_CONNECT_TIMEOUT", defaultDatabaseConnectTimeout)
	if err != nil {
		return Config{}, err
	}
	databasePingTimeout, err := getDurationEnvOrDefault(logger, "AVITO_KITCHEN_DATABASE_PING_TIMEOUT", defaultDatabasePingTimeout)
	if err != nil {
		return Config{}, err
	}
	readinessTimeout, err := getDurationEnvOrDefault(logger, "AVITO_KITCHEN_READINESS_TIMEOUT", defaultReadinessTimeout)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := getDurationEnvOrDefault(logger, "AVITO_KITCHEN_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	orderProcessorInterval, err := getDurationEnvOrDefault(logger, "AVITO_KITCHEN_ORDER_PROCESSOR_INTERVAL", defaultOrderProcessorInterval)
	if err != nil {
		return Config{}, err
	}
	cafeRequestTimeout, err := getDurationEnvOrDefault(logger, "AVITO_KITCHEN_CAFE_REQUEST_TIMEOUT", defaultCafeRequestTimeout)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddress:            getEnvOrDefault(logger, "AVITO_KITCHEN_HTTP_ADDR", defaultHTTPAddress),
		DatabaseURL:            getEnvOrDefault(logger, "AVITO_KITCHEN_DATABASE_URL", defaultDatabaseURL),
		DatabaseConnectTimeout: databaseConnectTimeout,
		DatabasePingTimeout:    databasePingTimeout,
		ReadinessTimeout:       readinessTimeout,
		ShutdownTimeout:        shutdownTimeout,
		OrderProcessorInterval: orderProcessorInterval,
		CafeRequestTimeout:     cafeRequestTimeout,
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
