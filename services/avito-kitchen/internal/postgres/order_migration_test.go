//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"
)

func TestOrderSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn := openTestDatabase(t, ctx)
	for _, relation := range []string{
		"orders",
		"order_items",
		"idempotency_keys",
		"idx_orders_processing_created_at",
	} {
		if !relationExists(t, ctx, conn, relation) {
			t.Errorf("relation %q does not exist; run docker compose up -d migrate", relation)
		}
	}
}

func TestOrderConstraints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn := openTestDatabase(t, ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin order constraint test transaction: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = tx.Rollback(cleanupCtx)
	})

	execValid(t, ctx, tx, `
		INSERT INTO restaurants (id, external_id, name, order_endpoint, timezone)
		VALUES ('01000000-0000-4000-8000-000000000001', 'order-cafe-1', 'Cafe', 'http://demo-cafe:8081/api/v1/orders', 'UTC')
	`)
	execValid(t, ctx, tx, `
		INSERT INTO menus (id, restaurant_id, snapshot_version)
		VALUES ('11000000-0000-4000-8000-000000000001', '01000000-0000-4000-8000-000000000001', 5)
	`)
	execValid(t, ctx, tx, `
		INSERT INTO menu_items (id, menu_id, external_id, name, price, available_quantity)
		VALUES ('21000000-0000-4000-8000-000000000001', '11000000-0000-4000-8000-000000000001', 'pizza-1', 'Pizza', 75000, 3)
	`)
	execValid(t, ctx, tx, `
		INSERT INTO orders (
			id, user_id, restaurant_id, menu_snapshot_version, status, total_price
		)
		VALUES (
			'31000000-0000-4000-8000-000000000001', 'user-1', '01000000-0000-4000-8000-000000000001', 5, 'PROCESSING', 150000
		)
	`)
	execValid(t, ctx, tx, `
		INSERT INTO order_items (
			id, order_id, menu_item_id, external_menu_item_id, name, unit_price, quantity
		)
		VALUES (
			'41000000-0000-4000-8000-000000000001', '31000000-0000-4000-8000-000000000001', '21000000-0000-4000-8000-000000000001', 'pizza-1', 'Pizza', 75000, 2
		)
	`)
	execValid(t, ctx, tx, `
		INSERT INTO idempotency_keys (operation, key, request_hash, order_id)
		VALUES ('CREATE_ORDER', 'key-1', repeat('a', 64), '31000000-0000-4000-8000-000000000001')
	`)

	tests := []struct {
		name     string
		query    string
		wantCode string
	}{
		{
			name: "order status must be known",
			query: `
				INSERT INTO orders (id, user_id, restaurant_id, menu_snapshot_version, status, total_price)
				VALUES ('31000000-0000-4000-8000-000000000002', 'user-2', '01000000-0000-4000-8000-000000000001', 5, 'UNKNOWN', 0)
			`,
			wantCode: "23514",
		},
		{
			name: "processing order cannot have rejection reason",
			query: `
				INSERT INTO orders (id, user_id, restaurant_id, menu_snapshot_version, status, rejection_reason, total_price)
				VALUES ('31000000-0000-4000-8000-000000000003', 'user-3', '01000000-0000-4000-8000-000000000001', 5, 'PROCESSING', 'OUT_OF_STOCK', 0)
			`,
			wantCode: "23514",
		},
		{
			name: "rejected order requires reason",
			query: `
				INSERT INTO orders (id, user_id, restaurant_id, menu_snapshot_version, status, total_price)
				VALUES ('31000000-0000-4000-8000-000000000004', 'user-4', '01000000-0000-4000-8000-000000000001', 5, 'REJECTED', 0)
			`,
			wantCode: "23514",
		},
		{
			name: "order total cannot be negative",
			query: `
				INSERT INTO orders (id, user_id, restaurant_id, menu_snapshot_version, status, total_price)
				VALUES ('31000000-0000-4000-8000-000000000005', 'user-5', '01000000-0000-4000-8000-000000000001', 5, 'PROCESSING', -1)
			`,
			wantCode: "23514",
		},
		{
			name: "order item quantity must be positive",
			query: `
				INSERT INTO order_items (id, order_id, menu_item_id, external_menu_item_id, name, unit_price, quantity)
				VALUES ('41000000-0000-4000-8000-000000000002', '31000000-0000-4000-8000-000000000001', '21000000-0000-4000-8000-000000000001', 'pizza-1', 'Pizza', 75000, 0)
			`,
			wantCode: "23514",
		},
		{
			name: "same menu item occurs once per order",
			query: `
				INSERT INTO order_items (id, order_id, menu_item_id, external_menu_item_id, name, unit_price, quantity)
				VALUES ('41000000-0000-4000-8000-000000000003', '31000000-0000-4000-8000-000000000001', '21000000-0000-4000-8000-000000000001', 'pizza-1', 'Pizza', 75000, 1)
			`,
			wantCode: "23505",
		},
		{
			name: "idempotency operation and key are unique",
			query: `
				INSERT INTO idempotency_keys (operation, key, request_hash)
				VALUES ('CREATE_ORDER', 'key-1', repeat('b', 64))
			`,
			wantCode: "23505",
		},
		{
			name: "idempotency order binding is unique",
			query: `
				INSERT INTO idempotency_keys (operation, key, request_hash, order_id)
				VALUES ('CREATE_ORDER', 'key-2', repeat('b', 64), '31000000-0000-4000-8000-000000000001')
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
