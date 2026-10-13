//go:build integration

package restaurant

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	restaurantservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/restaurant"
)

const defaultTestDatabaseURL = "postgres://avito:avito@127.0.0.1:5433/avito_kitchen?sslmode=disable"

func TestRepositoryListRestaurants(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openRepositoryTestTransaction(t, ctx)

	_, err := tx.Exec(ctx, `
		INSERT INTO restaurants (
			id,
			external_id,
			name,
			order_endpoint,
			timezone,
			is_accepting_orders,
			created_at
		)
		VALUES
			(
				'30000000-0000-4000-8000-000000000001',
				'repository-cafe-1',
				'Repository Cafe One',
				'http://demo-cafe:8081/api/v1/orders',
				'Europe/Moscow',
				true,
				'2026-08-28T10:00:00Z'
			),
			(
				'30000000-0000-4000-8000-000000000002',
				'repository-cafe-2',
				'Repository Cafe Two',
				'http://demo-cafe:8081/api/v1/orders',
				'UTC',
				false,
				'2026-08-28T11:00:00Z'
			)
	`)
	if err != nil {
		t.Fatalf("insert restaurants: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO restaurant_working_hours (
			restaurant_id,
			day_of_week,
			mode,
			opens_at,
			closes_at
		)
		VALUES
			('30000000-0000-4000-8000-000000000001', 1, 'INTERVAL', '09:30', '18:45'),
			('30000000-0000-4000-8000-000000000001', 2, 'OPEN_24H', NULL, NULL)
	`)
	if err != nil {
		t.Fatalf("insert working hours: %v", err)
	}

	repository := &Repository{database: tx}
	restaurants, err := repository.ListRestaurants(ctx)
	if err != nil {
		t.Fatalf("ListRestaurants() error = %v", err)
	}

	if len(restaurants) != 2 {
		t.Fatalf("restaurant count = %d, want 2", len(restaurants))
	}
	if restaurants[0].ID != "30000000-0000-4000-8000-000000000001" {
		t.Errorf("first restaurant ID = %q", restaurants[0].ID)
	}
	if restaurants[0].Name != "Repository Cafe One" || restaurants[0].Timezone != "Europe/Moscow" {
		t.Errorf("first restaurant = %#v", restaurants[0])
	}
	if !restaurants[0].IsAcceptingOrders {
		t.Error("first restaurant is not accepting orders")
	}
	if len(restaurants[0].WorkingHours) != 2 {
		t.Fatalf("first restaurant working hours count = %d, want 2", len(restaurants[0].WorkingHours))
	}
	if restaurants[0].WorkingHours[0].OpensAt != 9*time.Hour+30*time.Minute {
		t.Errorf("opens at = %s", restaurants[0].WorkingHours[0].OpensAt)
	}
	if restaurants[0].WorkingHours[0].ClosesAt != 18*time.Hour+45*time.Minute {
		t.Errorf("closes at = %s", restaurants[0].WorkingHours[0].ClosesAt)
	}
	if len(restaurants[1].WorkingHours) != 0 {
		t.Errorf("second restaurant working hours = %#v, want empty", restaurants[1].WorkingHours)
	}
}

func TestRepositoryCreateRestaurant(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	restaurant := restaurantservice.Restaurant{
		ID:                "50000000-0000-4000-8000-000000000001",
		ExternalID:        "create-repository-cafe",
		Name:              "Created Cafe",
		OrderEndpoint:     "http://demo-cafe:8081/api/v1/orders",
		Timezone:          "Europe/Moscow",
		IsAcceptingOrders: true,
		WorkingHours: []restaurantservice.WorkingHours{
			{
				DayOfWeek: 1,
				Mode:      restaurantservice.WorkingHoursInterval,
				OpensAt:   9*time.Hour + 15*time.Minute,
				ClosesAt:  18*time.Hour + 30*time.Minute,
			},
			{DayOfWeek: 2, Mode: restaurantservice.WorkingHoursOpen24H},
		},
	}

	created, err := repository.CreateRestaurant(ctx, restaurant)
	if err != nil {
		t.Fatalf("CreateRestaurant() error = %v", err)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Errorf("created timestamps = %s, %s", created.CreatedAt, created.UpdatedAt)
	}

	var restaurantCount int
	var workingHoursCount int
	var menuCount int

	err = tx.QueryRow(ctx, `
	SELECT
		count(DISTINCT r.id),
		count(DISTINCT h.day_of_week),
		count(DISTINCT m.id)
	FROM restaurants r
	LEFT JOIN restaurant_working_hours h
		ON h.restaurant_id = r.id
	LEFT JOIN menus m
		ON m.restaurant_id = r.id
	WHERE r.id = '50000000-0000-4000-8000-000000000001'
`).Scan(
		&restaurantCount,
		&workingHoursCount,
		&menuCount,
	)
	if err != nil {
		t.Fatalf("read created restaurant: %v", err)
	}

	if restaurantCount != 1 ||
		workingHoursCount != 2 ||
		menuCount != 1 {
		t.Errorf(
			"stored counts = restaurant:%d working_hours:%d menu:%d",
			restaurantCount,
			workingHoursCount,
			menuCount,
		)
	}
}

func TestRepositoryCreateRestaurantIsAtomic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	restaurant := restaurantservice.Restaurant{
		ID:                "50000000-0000-4000-8000-000000000003",
		ExternalID:        "atomic-repository-cafe",
		Name:              "Atomic Cafe",
		OrderEndpoint:     "http://demo-cafe:8081/api/v1/orders",
		Timezone:          "UTC",
		IsAcceptingOrders: true,
		WorkingHours: []restaurantservice.WorkingHours{
			{DayOfWeek: 1, Mode: restaurantservice.WorkingHoursOpen24H},
			{DayOfWeek: 1, Mode: restaurantservice.WorkingHoursClosed},
		},
	}

	_, err := repository.CreateRestaurant(ctx, restaurant)
	if err == nil {
		t.Fatal("CreateRestaurant() error = nil")
	}

	var count int
	err = tx.QueryRow(
		ctx,
		"SELECT count(*) FROM restaurants WHERE id = '50000000-0000-4000-8000-000000000003'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("check rolled back restaurant: %v", err)
	}
	if count != 0 {
		t.Errorf("restaurant count after failed schedule insert = %d, want 0", count)
	}
}

func openRepositoryTestTransaction(t *testing.T, ctx context.Context) pgx.Tx {
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

func TestRepositoryUpdateRestaurant(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}

	created, err := repository.CreateRestaurant(ctx, restaurantservice.Restaurant{
		ID:                "60000000-0000-4000-8000-000000000001",
		ExternalID:        "update-repository-cafe",
		Name:              "Original Cafe",
		OrderEndpoint:     "http://demo-cafe:8081/api/v1/orders",
		Timezone:          "Europe/Moscow",
		IsAcceptingOrders: true,
		WorkingHours: []restaurantservice.WorkingHours{
			{DayOfWeek: 1, Mode: restaurantservice.WorkingHoursOpen24H},
			{DayOfWeek: 2, Mode: restaurantservice.WorkingHoursClosed},
		},
	})
	if err != nil {
		t.Fatalf("CreateRestaurant() error = %v", err)
	}

	name := "Updated Cafe"
	isAccepting := false
	workingHours := []restaurantservice.WorkingHours{
		{
			DayOfWeek: 3,
			Mode:      restaurantservice.WorkingHoursInterval,
			OpensAt:   10 * time.Hour,
			ClosesAt:  20 * time.Hour,
		},
		{DayOfWeek: 7, Mode: restaurantservice.WorkingHoursOpen24H},
	}

	updated, err := repository.UpdateRestaurant(
		ctx,
		created.ID,
		restaurantservice.RestaurantUpdate{
			Name:              &name,
			IsAcceptingOrders: &isAccepting,
			WorkingHours:      &workingHours,
		},
	)
	if err != nil {
		t.Fatalf("UpdateRestaurant() error = %v", err)
	}

	if updated.Name != "Updated Cafe" {
		t.Errorf("name = %q", updated.Name)
	}
	if updated.IsAcceptingOrders {
		t.Error("is_accepting_orders = true, want false")
	}
	if updated.OrderEndpoint != "http://demo-cafe:8081/api/v1/orders" {
		t.Errorf("order endpoint changed unexpectedly: %q", updated.OrderEndpoint)
	}
	if updated.Timezone != "Europe/Moscow" {
		t.Errorf("timezone changed unexpectedly: %q", updated.Timezone)
	}
	if len(updated.WorkingHours) != 2 {
		t.Fatalf("working hours count = %d, want 2", len(updated.WorkingHours))
	}
	if updated.WorkingHours[0].DayOfWeek != 3 ||
		updated.WorkingHours[0].OpensAt != 10*time.Hour ||
		updated.WorkingHours[0].ClosesAt != 20*time.Hour {
		t.Errorf("first working hours = %#v", updated.WorkingHours[0])
	}
	if updated.WorkingHours[1].DayOfWeek != 7 ||
		updated.WorkingHours[1].Mode != restaurantservice.WorkingHoursOpen24H {
		t.Errorf("second working hours = %#v", updated.WorkingHours[1])
	}

	var oldScheduleCount int
	if err := tx.QueryRow(
		ctx,
		`SELECT count(*) FROM restaurant_working_hours WHERE restaurant_id = $1 AND day_of_week IN (1, 2)`,
		created.ID,
	).Scan(&oldScheduleCount); err != nil {
		t.Fatalf("check old working hours: %v", err)
	}
	if oldScheduleCount != 0 {
		t.Errorf("old working hours count = %d, want 0", oldScheduleCount)
	}
}

func TestRepositoryUpdateRestaurantNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}
	name := "Updated Cafe"

	_, err := repository.UpdateRestaurant(
		ctx,
		"60000000-0000-4000-8000-000000000099",
		restaurantservice.RestaurantUpdate{Name: &name},
	)
	if !errors.Is(err, restaurantservice.ErrRestaurantNotFound) {
		t.Fatalf("error = %v, want ErrRestaurantNotFound", err)
	}
}

func TestRepositoryUpdateRestaurantIsAtomic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx := openRepositoryTestTransaction(t, ctx)
	repository := &Repository{database: tx}

	created, err := repository.CreateRestaurant(ctx, restaurantservice.Restaurant{
		ID:                "60000000-0000-4000-8000-000000000002",
		ExternalID:        "atomic-update-repository-cafe",
		Name:              "Original Cafe",
		OrderEndpoint:     "http://demo-cafe:8081/api/v1/orders",
		Timezone:          "UTC",
		IsAcceptingOrders: true,
		WorkingHours: []restaurantservice.WorkingHours{
			{DayOfWeek: 1, Mode: restaurantservice.WorkingHoursOpen24H},
		},
	})
	if err != nil {
		t.Fatalf("CreateRestaurant() error = %v", err)
	}

	name := "Should Roll Back"
	invalidWorkingHours := []restaurantservice.WorkingHours{
		{DayOfWeek: 2, Mode: restaurantservice.WorkingHoursOpen24H},
		{DayOfWeek: 2, Mode: restaurantservice.WorkingHoursClosed},
	}
	_, err = repository.UpdateRestaurant(
		ctx,
		created.ID,
		restaurantservice.RestaurantUpdate{
			Name:         &name,
			WorkingHours: &invalidWorkingHours,
		},
	)
	if err == nil {
		t.Fatal("UpdateRestaurant() error = nil")
	}

	var storedName string
	if err := tx.QueryRow(ctx, `SELECT name FROM restaurants WHERE id = $1`, created.ID).Scan(&storedName); err != nil {
		t.Fatalf("read restaurant after rollback: %v", err)
	}
	if storedName != "Original Cafe" {
		t.Errorf("name after rollback = %q, want Original Cafe", storedName)
	}

	var (
		dayOfWeek int
		mode      string
	)
	if err := tx.QueryRow(
		ctx,
		`SELECT day_of_week, mode FROM restaurant_working_hours WHERE restaurant_id = $1`,
		created.ID,
	).Scan(&dayOfWeek, &mode); err != nil {
		t.Fatalf("read working hours after rollback: %v", err)
	}
	if dayOfWeek != 1 || mode != string(restaurantservice.WorkingHoursOpen24H) {
		t.Errorf("working hours after rollback = day:%d mode:%s", dayOfWeek, mode)
	}
}
