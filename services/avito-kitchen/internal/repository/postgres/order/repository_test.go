//go:build integration

package order

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

const (
	defaultTestDatabaseURL = "postgres://avito:avito@127.0.0.1:5433/avito_kitchen?sslmode=disable"
	testRestaurantID       = "72000000-0000-4000-8000-000000000001"
	testMenuID             = "82000000-0000-4000-8000-000000000001"
	testMenuItemID         = "92000000-0000-4000-8000-000000000001"
)

func TestRepositoryCreateOrderAndIdempotency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openOrderRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	insertOrderTestCatalog(t, ctx, tx)

	command := validOrderCommand("order-key-1", strings.Repeat("a", 64))
	created, err := repository.CreateOrder(ctx, command)
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if created.ID == "" || created.Status != orderservice.StatusProcessing {
		t.Errorf("created order = %#v", created)
	}
	if created.TotalPrice != 150000 || created.MenuSnapshotVersion != 5 {
		t.Errorf("total/version = %d/%d", created.TotalPrice, created.MenuSnapshotVersion)
	}
	if len(created.Items) != 1 {
		t.Fatalf("items count = %d", len(created.Items))
	}
	if created.Items[0].ExternalMenuItemID != "pizza-1" || created.Items[0].Name != "Margherita" || created.Items[0].UnitPrice != 75000 {
		t.Errorf("order item snapshot = %#v", created.Items[0])
	}

	var (
		orderCount       int
		orderItemCount   int
		idempotencyCount int
		boundOrderID     string
	)
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM orders WHERE id = $1`, created.ID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM order_items WHERE order_id = $1`, created.ID).Scan(&orderItemCount); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT count(*), max(order_id::text)
		FROM idempotency_keys
		WHERE operation = 'CREATE_ORDER' AND key = 'order-key-1'
	`).Scan(&idempotencyCount, &boundOrderID); err != nil {
		t.Fatal(err)
	}
	if orderCount != 1 || orderItemCount != 1 || idempotencyCount != 1 || boundOrderID != created.ID {
		t.Errorf(
			"stored state = orders:%d items:%d keys:%d bound:%s",
			orderCount,
			orderItemCount,
			idempotencyCount,
			boundOrderID,
		)
	}

	replayed, err := repository.CreateOrder(ctx, command)
	if err != nil {
		t.Fatalf("CreateOrder() retry error = %v", err)
	}
	if replayed.ID != created.ID || replayed.TotalPrice != created.TotalPrice || len(replayed.Items) != 1 {
		t.Errorf("replayed order = %#v, created = %#v", replayed, created)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM orders WHERE restaurant_id = $1`, testRestaurantID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 1 {
		t.Errorf("orders after retry = %d, want 1", orderCount)
	}

	reused := command
	reused.RequestHash = strings.Repeat("b", 64)
	_, err = repository.CreateOrder(ctx, reused)
	if !errors.Is(err, orderservice.ErrIdempotencyKeyReused) {
		t.Fatalf("reused key error = %v", err)
	}
}

func TestRepositoryCreateOrderRollsBackBusinessConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openOrderRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	insertOrderTestCatalog(t, ctx, tx)

	command := validOrderCommand("price-conflict-key", strings.Repeat("c", 64))
	command.Items[0].ExpectedPrice = 70000
	_, err := repository.CreateOrder(ctx, command)
	var priceError *orderservice.PriceChangedError
	if !errors.As(err, &priceError) {
		t.Fatalf("error = %v, want *PriceChangedError", err)
	}
	if len(priceError.Items) != 1 || priceError.Items[0].CurrentPrice != 75000 {
		t.Errorf("price error = %#v", priceError)
	}

	var orderCount, keyCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM orders WHERE restaurant_id = $1`, testRestaurantID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM idempotency_keys WHERE key = 'price-conflict-key'`).Scan(&keyCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 0 || keyCount != 0 {
		t.Errorf("state after rollback = orders:%d keys:%d, want 0/0", orderCount, keyCount)
	}
}

func TestRepositoryCreateOrderChecksCurrentState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tests := []struct {
		name    string
		prepare func(context.Context, pgx.Tx) error
		mutate  func(*orderservice.CreateOrderCommand)
		want    error
	}{
		{
			name: "restaurant not accepting orders",
			prepare: func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE restaurants SET is_accepting_orders = false WHERE id = $1`, testRestaurantID)
				return err
			},
			want: orderservice.ErrRestaurantUnavailable,
		},
		{
			name: "item inactive",
			prepare: func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE menu_items SET is_active = false WHERE id = $1`, testMenuItemID)
				return err
			},
			want: orderservice.ErrItemUnavailable,
		},
		{
			name:   "insufficient stock",
			mutate: func(command *orderservice.CreateOrderCommand) { command.Items[0].Quantity = 4 },
			want:   orderservice.ErrInsufficientStock,
		},
		{
			name: "item not found in restaurant menu",
			mutate: func(command *orderservice.CreateOrderCommand) {
				command.Items[0].MenuItemID = "92000000-0000-4000-8000-000000000099"
			},
			want: orderservice.ErrMenuItemNotFound,
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := openOrderRepositoryTestTransaction(t, ctx)
			repository := &Repository{database: tx}
			insertOrderTestCatalog(t, ctx, tx)
			if tt.prepare != nil {
				if err := tt.prepare(ctx, tx); err != nil {
					t.Fatal(err)
				}
			}
			command := validOrderCommand("state-key-"+string(rune('a'+index)), strings.Repeat("d", 64))
			if tt.mutate != nil {
				tt.mutate(&command)
			}

			_, err := repository.CreateOrder(ctx, command)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func openOrderRepositoryTestTransaction(t *testing.T, ctx context.Context) pgx.Tx {
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
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = conn.Close(cleanupCtx)
	})

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin order repository test transaction: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = tx.Rollback(cleanupCtx)
	})
	return tx
}

func insertOrderTestCatalog(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()

	if _, err := tx.Exec(ctx, `
		INSERT INTO restaurants (
			id, external_id, name, order_endpoint, timezone, is_accepting_orders
		)
		VALUES ($1, 'order-repository-cafe', 'Order Cafe', 'http://demo-cafe:8081/api/v1/orders', 'UTC', true)
	`, testRestaurantID); err != nil {
		t.Fatalf("insert restaurant: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO restaurant_working_hours (restaurant_id, day_of_week, mode)
		VALUES ($1, 1, 'OPEN_24H')
	`, testRestaurantID); err != nil {
		t.Fatalf("insert working hours: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO menus (id, restaurant_id, snapshot_version)
		VALUES ($1, $2, 5)
	`, testMenuID, testRestaurantID); err != nil {
		t.Fatalf("insert menu: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO menu_items (
			id, menu_id, external_id, name, price, available_quantity, is_active
		)
		VALUES ($1, $2, 'pizza-1', 'Margherita', 75000, 3, true)
	`, testMenuItemID, testMenuID); err != nil {
		t.Fatalf("insert menu item: %v", err)
	}
}

func validOrderCommand(key, requestHash string) orderservice.CreateOrderCommand {
	return orderservice.CreateOrderCommand{
		IdempotencyKey: key,
		RequestHash:    requestHash,
		UserID:         "user-1",
		RestaurantID:   testRestaurantID,
		Items: []orderservice.CreateOrderItemInput{
			{MenuItemID: testMenuItemID, Quantity: 2, ExpectedPrice: 75000},
		},
		Now: time.Date(2026, time.August, 31, 10, 0, 0, 0, time.UTC),
	}
}

func TestRepositoryGetOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openOrderRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	insertOrderTestCatalog(t, ctx, tx)

	created, err := repository.CreateOrder(ctx, validOrderCommand("get-order-key", strings.Repeat("e", 64)))
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}

	got, err := repository.GetOrder(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}
	if got.ID != created.ID || got.UserID != created.UserID || got.RestaurantID != created.RestaurantID {
		t.Errorf("GetOrder() identity = %#v, created = %#v", got, created)
	}
	if got.Status != orderservice.StatusProcessing || got.TotalPrice != 150000 || got.MenuSnapshotVersion != 5 {
		t.Errorf("GetOrder() state = %#v", got)
	}
	if got.ExternalOrderID != nil || got.RejectionReason != nil {
		t.Errorf("GetOrder() optional fields = external:%v rejection:%v, want nil/nil", got.ExternalOrderID, got.RejectionReason)
	}
	if len(got.Items) != 1 {
		t.Fatalf("GetOrder() items count = %d", len(got.Items))
	}
	if got.Items[0].MenuItemID != testMenuItemID || got.Items[0].ExternalMenuItemID != "pizza-1" || got.Items[0].Name != "Margherita" || got.Items[0].UnitPrice != 75000 || got.Items[0].Quantity != 2 {
		t.Errorf("GetOrder() item = %#v", got.Items[0])
	}
}

func TestRepositoryGetOrderNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openOrderRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}

	_, err := repository.GetOrder(ctx, "30000000-0000-4000-8000-000000000099")
	if !errors.Is(err, orderservice.ErrOrderNotFound) {
		t.Fatalf("GetOrder() error = %v, want ErrOrderNotFound", err)
	}
}

func TestRepositoryProcessingOrdersAndFinalization(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openOrderRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	insertOrderTestCatalog(t, ctx, tx)

	accepted, err := repository.CreateOrder(ctx, validOrderCommand("delivery-accepted", strings.Repeat("f", 64)))
	if err != nil {
		t.Fatalf("CreateOrder() accepted error = %v", err)
	}

	processing, err := repository.ListProcessingOrders(ctx)
	if err != nil {
		t.Fatalf("ListProcessingOrders() error = %v", err)
	}
	if len(processing) != 1 {
		t.Fatalf("processing count = %d, want 1", len(processing))
	}
	got := processing[0]
	if got.ID != accepted.ID || got.RestaurantExternalID != "order-repository-cafe" {
		t.Errorf("delivery identity = %#v", got)
	}
	if got.OrderEndpoint != "http://demo-cafe:8081/api/v1/orders" || got.MenuSnapshotVersion != 5 {
		t.Errorf("delivery route/version = %#v", got)
	}
	if len(got.Items) != 1 || got.Items[0].ExternalMenuItemID != "pizza-1" || got.Items[0].Quantity != 2 {
		t.Errorf("delivery items = %#v", got.Items)
	}

	if err := repository.MarkAccepted(ctx, accepted.ID, "cafe-order-1"); err != nil {
		t.Fatalf("MarkAccepted() error = %v", err)
	}
	acceptedState, err := repository.GetOrder(ctx, accepted.ID)
	if err != nil {
		t.Fatalf("GetOrder() accepted error = %v", err)
	}
	if acceptedState.Status != orderservice.StatusAccepted || acceptedState.ExternalOrderID == nil || *acceptedState.ExternalOrderID != "cafe-order-1" {
		t.Errorf("accepted state = %#v", acceptedState)
	}
	if acceptedState.RejectionReason != nil {
		t.Errorf("accepted rejection reason = %v, want nil", acceptedState.RejectionReason)
	}

	// Finalization is idempotent: a late duplicate result must not overwrite a final state.
	if err := repository.MarkRejected(ctx, accepted.ID, orderservice.RejectionReasonOutOfStock); err != nil {
		t.Fatalf("MarkRejected() after accepted error = %v", err)
	}
	acceptedState, err = repository.GetOrder(ctx, accepted.ID)
	if err != nil {
		t.Fatalf("GetOrder() after duplicate finalization error = %v", err)
	}
	if acceptedState.Status != orderservice.StatusAccepted {
		t.Errorf("status after duplicate finalization = %s, want ACCEPTED", acceptedState.Status)
	}

	rejected, err := repository.CreateOrder(ctx, validOrderCommand("delivery-rejected", strings.Repeat("1", 64)))
	if err != nil {
		t.Fatalf("CreateOrder() rejected error = %v", err)
	}
	if err := repository.MarkRejected(ctx, rejected.ID, orderservice.RejectionReasonItemUnavailable); err != nil {
		t.Fatalf("MarkRejected() error = %v", err)
	}
	rejectedState, err := repository.GetOrder(ctx, rejected.ID)
	if err != nil {
		t.Fatalf("GetOrder() rejected error = %v", err)
	}
	if rejectedState.Status != orderservice.StatusRejected || rejectedState.RejectionReason == nil || *rejectedState.RejectionReason != orderservice.RejectionReasonItemUnavailable {
		t.Errorf("rejected state = %#v", rejectedState)
	}
	if rejectedState.ExternalOrderID != nil {
		t.Errorf("rejected external order id = %v, want nil", rejectedState.ExternalOrderID)
	}

	processing, err = repository.ListProcessingOrders(ctx)
	if err != nil {
		t.Fatalf("ListProcessingOrders() after finalization error = %v", err)
	}
	if len(processing) != 0 {
		t.Errorf("processing after finalization = %#v, want empty", processing)
	}
}
