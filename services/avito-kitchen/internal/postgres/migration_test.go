//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const defaultTestDatabaseURL = "postgres://avito:avito@127.0.0.1:5433/avito_kitchen?sslmode=disable"

func TestCatalogSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn := openTestDatabase(t, ctx)

	for _, relation := range []string{
		"restaurants",
		"restaurant_working_hours",
		"menus",
		"menu_items",
		"idx_menu_items_active",
	} {
		if !relationExists(t, ctx, conn, relation) {
			t.Errorf("relation %q does not exist; run docker compose up -d migrate", relation)
		}
	}
}

func TestCatalogConstraints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn := openTestDatabase(t, ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin catalog constraint test transaction: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = tx.Rollback(cleanupCtx)
	})

	execValid(t, ctx, tx, `
		INSERT INTO restaurants (id, external_id, name, order_endpoint, timezone)
		VALUES
			('00000000-0000-4000-8000-000000000001', 'cafe-1', 'Cafe One', 'http://demo-cafe:8081/api/v1/orders', 'Europe/Moscow'),
			('00000000-0000-4000-8000-000000000002', 'cafe-2', 'Cafe Two', 'http://demo-cafe:8081/api/v1/orders', 'Europe/Moscow')
	`)
	execValid(t, ctx, tx, `
		INSERT INTO restaurant_working_hours (restaurant_id, day_of_week, mode, opens_at, closes_at)
		VALUES
			('00000000-0000-4000-8000-000000000001', 1, 'OPEN_24H', NULL, NULL),
			('00000000-0000-4000-8000-000000000001', 2, 'INTERVAL', '09:00', '22:00')
	`)
	execValid(t, ctx, tx, `
		INSERT INTO menus (id, restaurant_id)
		VALUES ('10000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000001')
	`)
	execValid(t, ctx, tx, `
		INSERT INTO menu_items (id, menu_id, external_id, name, price, available_quantity)
		VALUES ('20000000-0000-4000-8000-000000000001', '10000000-0000-4000-8000-000000000001', 'pizza-1', 'Margherita', 75000, 10)
	`)

	var acceptingOrders bool
	var snapshotVersion int64
	var itemActive bool
	err = tx.QueryRow(ctx, `
		SELECT r.is_accepting_orders, m.snapshot_version, mi.is_active
		FROM restaurants r
		JOIN menus m ON m.restaurant_id = r.id
		JOIN menu_items mi ON mi.menu_id = m.id
		WHERE r.id = '00000000-0000-4000-8000-000000000001'
	`).Scan(&acceptingOrders, &snapshotVersion, &itemActive)
	if err != nil {
		t.Fatalf("read catalog defaults: %v", err)
	}
	if !acceptingOrders || snapshotVersion != 0 || !itemActive {
		t.Errorf(
			"defaults = accepting_orders:%t snapshot_version:%d item_active:%t",
			acceptingOrders,
			snapshotVersion,
			itemActive,
		)
	}

	tests := []struct {
		name     string
		query    string
		wantCode string
	}{
		{
			name: "restaurant external ID is unique",
			query: `
				INSERT INTO restaurants (id, external_id, name, order_endpoint, timezone)
				VALUES ('00000000-0000-4000-8000-000000000003', 'cafe-1', 'Duplicate', 'http://demo-cafe:8081/api/v1/orders', 'Europe/Moscow')
			`,
			wantCode: "23505",
		},
		{
			name: "weekday must be in ISO range",
			query: `
				INSERT INTO restaurant_working_hours (restaurant_id, day_of_week, mode)
				VALUES ('00000000-0000-4000-8000-000000000001', 0, 'OPEN_24H')
			`,
			wantCode: "23514",
		},
		{
			name: "working hours mode must be known",
			query: `
				INSERT INTO restaurant_working_hours (restaurant_id, day_of_week, mode)
				VALUES ('00000000-0000-4000-8000-000000000001', 3, 'UNKNOWN')
			`,
			wantCode: "23514",
		},
		{
			name: "interval requires both boundaries",
			query: `
				INSERT INTO restaurant_working_hours (restaurant_id, day_of_week, mode)
				VALUES ('00000000-0000-4000-8000-000000000001', 4, 'INTERVAL')
			`,
			wantCode: "23514",
		},
		{
			name: "interval cannot cross midnight",
			query: `
				INSERT INTO restaurant_working_hours (restaurant_id, day_of_week, mode, opens_at, closes_at)
				VALUES ('00000000-0000-4000-8000-000000000001', 5, 'INTERVAL', '18:00', '03:00')
			`,
			wantCode: "23514",
		},
		{
			name: "closed day cannot contain boundaries",
			query: `
				INSERT INTO restaurant_working_hours (restaurant_id, day_of_week, mode, opens_at, closes_at)
				VALUES ('00000000-0000-4000-8000-000000000001', 6, 'CLOSED', '09:00', '22:00')
			`,
			wantCode: "23514",
		},
		{
			name: "working hours require a restaurant",
			query: `
				INSERT INTO restaurant_working_hours (restaurant_id, day_of_week, mode)
				VALUES ('90000000-0000-4000-8000-000000000001', 1, 'OPEN_24H')
			`,
			wantCode: "23503",
		},
		{
			name: "restaurant has one current menu",
			query: `
				INSERT INTO menus (id, restaurant_id)
				VALUES ('10000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000001')
			`,
			wantCode: "23505",
		},
		{
			name: "snapshot version cannot be negative",
			query: `
				INSERT INTO menus (id, restaurant_id, snapshot_version)
				VALUES ('10000000-0000-4000-8000-000000000003', '00000000-0000-4000-8000-000000000002', -1)
			`,
			wantCode: "23514",
		},
		{
			name: "menu item price cannot be negative",
			query: `
				INSERT INTO menu_items (id, menu_id, external_id, name, price, available_quantity)
				VALUES ('20000000-0000-4000-8000-000000000002', '10000000-0000-4000-8000-000000000001', 'negative-price', 'Invalid', -1, 1)
			`,
			wantCode: "23514",
		},
		{
			name: "available quantity cannot be negative",
			query: `
				INSERT INTO menu_items (id, menu_id, external_id, name, price, available_quantity)
				VALUES ('20000000-0000-4000-8000-000000000003', '10000000-0000-4000-8000-000000000001', 'negative-quantity', 'Invalid', 100, -1)
			`,
			wantCode: "23514",
		},
		{
			name: "external item ID is unique within menu",
			query: `
				INSERT INTO menu_items (id, menu_id, external_id, name, price, available_quantity)
				VALUES ('20000000-0000-4000-8000-000000000004', '10000000-0000-4000-8000-000000000001', 'pizza-1', 'Duplicate', 100, 1)
			`,
			wantCode: "23505",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectConstraintViolation(t, ctx, tx, tt.query, tt.wantCode)
		})
	}
}

func openTestDatabase(t *testing.T, ctx context.Context) *pgx.Conn {
	t.Helper()

	databaseURL := os.Getenv("AVITO_KITCHEN_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultTestDatabaseURL
	}

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to integration test PostgreSQL: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = conn.Close(cleanupCtx)
	})

	return conn
}

func relationExists(t *testing.T, ctx context.Context, conn *pgx.Conn, name string) bool {
	t.Helper()

	var exists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists); err != nil {
		t.Fatalf("check relation %s: %v", name, err)
	}
	return exists
}

func execValid(t *testing.T, ctx context.Context, tx pgx.Tx, query string) {
	t.Helper()

	if _, err := tx.Exec(ctx, query); err != nil {
		t.Fatalf("execute valid catalog query: %v", err)
	}
}

func expectConstraintViolation(t *testing.T, ctx context.Context, tx pgx.Tx, query, wantCode string) {
	t.Helper()

	savepoint, err := tx.Begin(ctx)
	if err != nil {
		t.Fatalf("create constraint test savepoint: %v", err)
	}

	_, queryErr := savepoint.Exec(ctx, query)
	if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
		t.Fatalf("rollback constraint test savepoint: %v", rollbackErr)
	}
	if queryErr == nil {
		t.Fatal("constraint violation error = nil")
	}

	var postgresError *pgconn.PgError
	if !errors.As(queryErr, &postgresError) {
		t.Fatalf("error type = %T, want *pgconn.PgError", queryErr)
	}
	if postgresError.Code != wantCode {
		t.Errorf("PostgreSQL error code = %s, want %s", postgresError.Code, wantCode)
	}
}
