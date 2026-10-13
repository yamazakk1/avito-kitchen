package menu

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	menuservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/menu"
)

const getMenuQuery = `
	SELECT
		id,
		snapshot_version,
		updated_at
	FROM menus
	WHERE restaurant_id = $1
`

const listActiveMenuItemsQuery = `
	SELECT
		id,
		external_id,
		name,
		description,
		price,
		available_quantity
	FROM menu_items
	WHERE menu_id = $1
		AND is_active = true
	ORDER BY created_at, id
`

const lockMenuQuery = `
	SELECT
		id,
		snapshot_version,
		updated_at
	FROM menus
	WHERE restaurant_id = $1
	FOR UPDATE
`

const deactivateMenuItemsQuery = `
	UPDATE menu_items
	SET
		is_active = false,
		updated_at = now()
	WHERE menu_id = $1
		AND is_active = true
`

const upsertMenuItemQuery = `
	INSERT INTO menu_items (
		id,
		menu_id,
		external_id,
		name,
		description,
		price,
		available_quantity,
		is_active
	)
	VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, true)
	ON CONFLICT (menu_id, external_id)
	DO UPDATE SET
		name = EXCLUDED.name,
		description = EXCLUDED.description,
		price = EXCLUDED.price,
		available_quantity = EXCLUDED.available_quantity,
		is_active = true,
		updated_at = now()
`

const updateMenuVersionQuery = `
	UPDATE menus
	SET
		snapshot_version = $2,
		updated_at = now()
	WHERE id = $1
	RETURNING updated_at
`

const countActiveMenuItemsQuery = `
	SELECT count(*)
	FROM menu_items
	WHERE menu_id = $1
		AND is_active = true
`

const updateAvailabilityQuery = `
	UPDATE menu_items
	SET
		available_quantity = $3,
		updated_at = now()
	WHERE menu_id = $1
		AND external_id = $2
		AND is_active = true
	RETURNING available_quantity, updated_at
`

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type database interface {
	queryer
	Begin(context.Context) (pgx.Tx, error)
}

type Repository struct {
	database database
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{database: pool}
}

func (r *Repository) GetMenu(ctx context.Context, restaurantID string) (menuservice.Menu, error) {
	var (
		menuID          string
		snapshotVersion int64
		updatedAt       time.Time
	)
	if err := r.database.QueryRow(ctx, getMenuQuery, restaurantID).Scan(
		&menuID,
		&snapshotVersion,
		&updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return menuservice.Menu{}, menuservice.ErrRestaurantNotFound
		}
		return menuservice.Menu{}, fmt.Errorf("query menu: %w", err)
	}

	items, err := loadActiveMenuItems(ctx, r.database, menuID)
	if err != nil {
		return menuservice.Menu{}, err
	}

	return menuservice.Menu{
		RestaurantID:    restaurantID,
		SnapshotVersion: snapshotVersion,
		UpdatedAt:       updatedAt,
		Items:           items,
	}, nil
}

func (r *Repository) ReplaceSnapshot(
	ctx context.Context,
	restaurantID string,
	snapshot menuservice.Snapshot,
) (menuservice.SnapshotResult, error) {
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return menuservice.SnapshotResult{}, fmt.Errorf("begin replace menu snapshot transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		menuID           string
		currentVersion   int64
		currentUpdatedAt time.Time
	)
	if err := tx.QueryRow(ctx, lockMenuQuery, restaurantID).Scan(
		&menuID,
		&currentVersion,
		&currentUpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return menuservice.SnapshotResult{}, menuservice.ErrRestaurantNotFound
		}
		return menuservice.SnapshotResult{}, fmt.Errorf("lock menu: %w", err)
	}

	if snapshot.Version < currentVersion {
		return menuservice.SnapshotResult{}, &menuservice.StaleSnapshotError{
			CurrentVersion:  currentVersion,
			ReceivedVersion: snapshot.Version,
		}
	}

	if snapshot.Version == currentVersion {
		itemsCount, err := countActiveMenuItems(ctx, tx, menuID)
		if err != nil {
			return menuservice.SnapshotResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return menuservice.SnapshotResult{}, fmt.Errorf("commit repeated menu snapshot transaction: %w", err)
		}
		return menuservice.SnapshotResult{
			SnapshotVersion: currentVersion,
			UpdatedAt:       currentUpdatedAt,
			ItemsCount:      itemsCount,
		}, nil
	}

	if _, err := tx.Exec(ctx, deactivateMenuItemsQuery, menuID); err != nil {
		return menuservice.SnapshotResult{}, fmt.Errorf("deactivate previous menu items: %w", err)
	}

	for _, item := range snapshot.Items {
		if _, err := tx.Exec(
			ctx,
			upsertMenuItemQuery,
			menuID,
			item.ExternalID,
			item.Name,
			item.Description,
			item.Price,
			item.AvailableQuantity,
		); err != nil {
			return menuservice.SnapshotResult{}, fmt.Errorf("upsert menu item %q: %w", item.ExternalID, err)
		}
	}

	var updatedAt time.Time
	if err := tx.QueryRow(ctx, updateMenuVersionQuery, menuID, snapshot.Version).Scan(&updatedAt); err != nil {
		return menuservice.SnapshotResult{}, fmt.Errorf("update menu snapshot version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return menuservice.SnapshotResult{}, fmt.Errorf("commit replace menu snapshot transaction: %w", err)
	}

	return menuservice.SnapshotResult{
		SnapshotVersion: snapshot.Version,
		UpdatedAt:       updatedAt,
		ItemsCount:      len(snapshot.Items),
	}, nil
}

func (r *Repository) UpdateAvailability(
	ctx context.Context,
	restaurantID string,
	externalItemID string,
	availableQuantity int,
) (menuservice.AvailabilityResult, error) {
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return menuservice.AvailabilityResult{}, fmt.Errorf("begin update availability transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		menuID           string
		currentVersion   int64
		currentUpdatedAt time.Time
	)
	if err := tx.QueryRow(ctx, lockMenuQuery, restaurantID).Scan(
		&menuID,
		&currentVersion,
		&currentUpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return menuservice.AvailabilityResult{}, menuservice.ErrRestaurantNotFound
		}
		return menuservice.AvailabilityResult{}, fmt.Errorf("lock menu for availability update: %w", err)
	}

	var result menuservice.AvailabilityResult
	result.ExternalItemID = externalItemID
	if err := tx.QueryRow(
		ctx,
		updateAvailabilityQuery,
		menuID,
		externalItemID,
		availableQuantity,
	).Scan(&result.AvailableQuantity, &result.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return menuservice.AvailabilityResult{}, menuservice.ErrMenuItemNotFound
		}
		return menuservice.AvailabilityResult{}, fmt.Errorf("update menu item availability: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return menuservice.AvailabilityResult{}, fmt.Errorf("commit update availability transaction: %w", err)
	}
	return result, nil
}

func loadActiveMenuItems(
	ctx context.Context,
	queryer queryer,
	menuID string,
) ([]menuservice.Item, error) {
	rows, err := queryer.Query(ctx, listActiveMenuItemsQuery, menuID)
	if err != nil {
		return nil, fmt.Errorf("query active menu items: %w", err)
	}
	defer rows.Close()

	items := make([]menuservice.Item, 0)
	for rows.Next() {
		var (
			item        menuservice.Item
			description pgtype.Text
		)
		if err := rows.Scan(
			&item.ID,
			&item.ExternalID,
			&item.Name,
			&description,
			&item.Price,
			&item.AvailableQuantity,
		); err != nil {
			return nil, fmt.Errorf("scan menu item: %w", err)
		}
		if description.Valid {
			value := description.String
			item.Description = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate menu items: %w", err)
	}
	return items, nil
}

func countActiveMenuItems(ctx context.Context, queryer queryer, menuID string) (int, error) {
	var count int
	if err := queryer.QueryRow(ctx, countActiveMenuItemsQuery, menuID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active menu items: %w", err)
	}
	return count, nil
}

var _ menuservice.Repository = (*Repository)(nil)
var _ database = (*pgxpool.Pool)(nil)
var _ database = (pgx.Tx)(nil)
