//go:build integration

package menu

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	menuservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/menu"
)

const defaultTestDatabaseURL = "postgres://avito:avito@127.0.0.1:5433/avito_kitchen?sslmode=disable"

func TestRepositoryReplaceAndGetMenu(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openMenuRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	insertRestaurantWithMenu(t, ctx, tx, "70000000-0000-4000-8000-000000000001", "menu-repository-cafe")

	result, err := repository.ReplaceSnapshot(ctx, "70000000-0000-4000-8000-000000000001", menuservice.Snapshot{
		Version: 1,
		Items: []menuservice.SnapshotItem{
			{ExternalID: "pizza-1", Name: "Margherita", Price: 75000, AvailableQuantity: 5},
			{ExternalID: "cola-1", Name: "Cola", Price: 15000, AvailableQuantity: 10},
		},
	})
	if err != nil {
		t.Fatalf("ReplaceSnapshot() first error = %v", err)
	}
	if result.SnapshotVersion != 1 || result.ItemsCount != 2 || result.UpdatedAt.IsZero() {
		t.Errorf("first result = %#v", result)
	}

	description := "Updated recipe"
	result, err = repository.ReplaceSnapshot(ctx, "70000000-0000-4000-8000-000000000001", menuservice.Snapshot{
		Version: 3,
		Items: []menuservice.SnapshotItem{
			{ExternalID: "pizza-1", Name: "Margherita XL", Description: &description, Price: 90000, AvailableQuantity: 2},
			{ExternalID: "water-1", Name: "Water", Price: 10000, AvailableQuantity: 20},
		},
	})
	if err != nil {
		t.Fatalf("ReplaceSnapshot() second error = %v", err)
	}
	if result.SnapshotVersion != 3 || result.ItemsCount != 2 {
		t.Errorf("second result = %#v", result)
	}

	menu, err := repository.GetMenu(ctx, "70000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatalf("GetMenu() error = %v", err)
	}
	if menu.SnapshotVersion != 3 || len(menu.Items) != 2 {
		t.Fatalf("menu = %#v", menu)
	}
	if menu.Items[0].ExternalID != "pizza-1" || menu.Items[0].Name != "Margherita XL" || menu.Items[0].Price != 90000 {
		t.Errorf("updated pizza = %#v", menu.Items[0])
	}
	if menu.Items[0].Description == nil || *menu.Items[0].Description != description {
		t.Errorf("description = %#v", menu.Items[0].Description)
	}

	var colaActive bool
	if err := tx.QueryRow(ctx, `
		SELECT is_active
		FROM menu_items mi
		JOIN menus m ON m.id = mi.menu_id
		WHERE m.restaurant_id = $1 AND mi.external_id = 'cola-1'
	`, "70000000-0000-4000-8000-000000000001").Scan(&colaActive); err != nil {
		t.Fatalf("read removed item state: %v", err)
	}
	if colaActive {
		t.Error("removed item remains active")
	}
}

func TestRepositoryReplaceSnapshotVersionRules(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openMenuRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	insertRestaurantWithMenu(t, ctx, tx, "70000000-0000-4000-8000-000000000002", "version-repository-cafe")

	first, err := repository.ReplaceSnapshot(ctx, "70000000-0000-4000-8000-000000000002", menuservice.Snapshot{
		Version: 5,
		Items:   []menuservice.SnapshotItem{{ExternalID: "pizza", Name: "Pizza", Price: 100, AvailableQuantity: 1}},
	})
	if err != nil {
		t.Fatalf("initial ReplaceSnapshot() error = %v", err)
	}

	retry, err := repository.ReplaceSnapshot(ctx, "70000000-0000-4000-8000-000000000002", menuservice.Snapshot{
		Version: 5,
		Items:   []menuservice.SnapshotItem{{ExternalID: "different", Name: "Ignored", Price: 999, AvailableQuantity: 9}},
	})
	if err != nil {
		t.Fatalf("retry ReplaceSnapshot() error = %v", err)
	}
	if retry.SnapshotVersion != 5 || retry.ItemsCount != 1 || !retry.UpdatedAt.Equal(first.UpdatedAt) {
		t.Errorf("retry result = %#v, first = %#v", retry, first)
	}

	_, err = repository.ReplaceSnapshot(ctx, "70000000-0000-4000-8000-000000000002", menuservice.Snapshot{Version: 4})
	if !errors.Is(err, menuservice.ErrStaleSnapshotUpdate) {
		t.Fatalf("stale error = %v, want ErrStaleSnapshotUpdate", err)
	}
	var stale *menuservice.StaleSnapshotError
	if !errors.As(err, &stale) || stale.CurrentVersion != 5 || stale.ReceivedVersion != 4 {
		t.Errorf("stale details = %#v", stale)
	}
}

func TestRepositoryReplaceSnapshotIsAtomic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openMenuRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	insertRestaurantWithMenu(t, ctx, tx, "70000000-0000-4000-8000-000000000003", "atomic-menu-cafe")

	_, err := repository.ReplaceSnapshot(ctx, "70000000-0000-4000-8000-000000000003", menuservice.Snapshot{
		Version: 1,
		Items:   []menuservice.SnapshotItem{{ExternalID: "pizza", Name: "Pizza", Price: 100, AvailableQuantity: 1}},
	})
	if err != nil {
		t.Fatalf("initial ReplaceSnapshot() error = %v", err)
	}

	_, err = repository.ReplaceSnapshot(ctx, "70000000-0000-4000-8000-000000000003", menuservice.Snapshot{
		Version: 2,
		Items:   []menuservice.SnapshotItem{{ExternalID: "broken", Name: "Broken", Price: -1, AvailableQuantity: 1}},
	})
	if err == nil {
		t.Fatal("invalid repository snapshot error = nil")
	}

	menu, err := repository.GetMenu(ctx, "70000000-0000-4000-8000-000000000003")
	if err != nil {
		t.Fatalf("GetMenu() after rollback error = %v", err)
	}
	if menu.SnapshotVersion != 1 || len(menu.Items) != 1 || menu.Items[0].ExternalID != "pizza" {
		t.Errorf("menu after rollback = %#v", menu)
	}
}

func TestRepositoryUpdateAvailability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openMenuRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	insertRestaurantWithMenu(t, ctx, tx, "70000000-0000-4000-8000-000000000004", "availability-menu-cafe")

	_, err := repository.ReplaceSnapshot(ctx, "70000000-0000-4000-8000-000000000004", menuservice.Snapshot{
		Version: 1,
		Items:   []menuservice.SnapshotItem{{ExternalID: "pizza", Name: "Pizza", Price: 100, AvailableQuantity: 5}},
	})
	if err != nil {
		t.Fatalf("ReplaceSnapshot() error = %v", err)
	}

	result, err := repository.UpdateAvailability(ctx, "70000000-0000-4000-8000-000000000004", "pizza", 2)
	if err != nil {
		t.Fatalf("UpdateAvailability() error = %v", err)
	}
	if result.ExternalItemID != "pizza" || result.AvailableQuantity != 2 || result.UpdatedAt.IsZero() {
		t.Errorf("result = %#v", result)
	}

	menu, err := repository.GetMenu(ctx, "70000000-0000-4000-8000-000000000004")
	if err != nil {
		t.Fatalf("GetMenu() error = %v", err)
	}
	if menu.SnapshotVersion != 1 {
		t.Errorf("snapshot version changed after availability update: %d", menu.SnapshotVersion)
	}
	if len(menu.Items) != 1 || menu.Items[0].AvailableQuantity != 2 {
		t.Errorf("menu items = %#v", menu.Items)
	}

	_, err = repository.UpdateAvailability(ctx, "70000000-0000-4000-8000-000000000004", "missing", 1)
	if !errors.Is(err, menuservice.ErrMenuItemNotFound) {
		t.Errorf("missing item error = %v", err)
	}

	_, err = repository.UpdateAvailability(ctx, "70000000-0000-4000-8000-000000000099", "pizza", 1)
	if !errors.Is(err, menuservice.ErrRestaurantNotFound) {
		t.Errorf("missing restaurant error = %v", err)
	}
}

func openMenuRepositoryTestTransaction(t *testing.T, ctx context.Context) pgx.Tx {
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
		t.Fatalf("begin repository test transaction: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = tx.Rollback(cleanupCtx)
	})
	return tx
}

func insertRestaurantWithMenu(t *testing.T, ctx context.Context, tx pgx.Tx, restaurantID, externalID string) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
		INSERT INTO restaurants (id, external_id, name, order_endpoint, timezone)
		VALUES ($1, $2, 'Menu Cafe', 'http://demo-cafe:8081/api/v1/orders', 'UTC')
	`, restaurantID, externalID); err != nil {
		t.Fatalf("insert restaurant: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO menus (id, restaurant_id)
		VALUES (gen_random_uuid(), $1)
	`, restaurantID); err != nil {
		t.Fatalf("insert menu: %v", err)
	}
}
