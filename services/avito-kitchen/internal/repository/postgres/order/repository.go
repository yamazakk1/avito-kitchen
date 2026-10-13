package order

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	orderservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/order"
)

const createOrderOperation = "CREATE_ORDER"

const claimIdempotencyKeyQuery = `
	INSERT INTO idempotency_keys (
		operation,
		key,
		request_hash
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (operation, key) DO NOTHING
`

const getIdempotencyKeyQuery = `
	SELECT
		request_hash,
		COALESCE(order_id::text, '')
	FROM idempotency_keys
	WHERE operation = $1
		AND key = $2
`

const lockRestaurantQuery = `
	SELECT
		timezone,
		is_accepting_orders
	FROM restaurants
	WHERE id = $1
	FOR UPDATE
`

const listWorkingHoursQuery = `
	SELECT
		day_of_week,
		mode,
		opens_at,
		closes_at
	FROM restaurant_working_hours
	WHERE restaurant_id = $1
	ORDER BY day_of_week
`

const lockMenuQuery = `
	SELECT
		id::text,
		snapshot_version
	FROM menus
	WHERE restaurant_id = $1
	FOR UPDATE
`

const insertOrderQuery = `
	INSERT INTO orders (
		id,
		user_id,
		restaurant_id,
		menu_snapshot_version,
		status,
		total_price
	)
	VALUES (
		gen_random_uuid(),
		$1,
		$2,
		$3,
		$4,
		$5
	)
	RETURNING id::text, created_at, updated_at
`

const insertOrderItemQuery = `
	INSERT INTO order_items (
		id,
		order_id,
		menu_item_id,
		external_menu_item_id,
		name,
		unit_price,
		quantity
	)
	VALUES (
		gen_random_uuid(),
		$1,
		$2,
		$3,
		$4,
		$5,
		$6
	)
`

const bindIdempotencyKeyQuery = `
	UPDATE idempotency_keys
	SET order_id = $3
	WHERE operation = $1
		AND key = $2
`

const getOrderQuery = `
	SELECT
		id::text,
		user_id,
		restaurant_id::text,
		external_order_id,
		menu_snapshot_version,
		status,
		rejection_reason,
		total_price,
		created_at,
		updated_at
	FROM orders
	WHERE id = $1
`

const listOrderItemsQuery = `
	SELECT
		menu_item_id::text,
		external_menu_item_id,
		name,
		unit_price,
		quantity
	FROM order_items
	WHERE order_id = $1
	ORDER BY created_at, id
`

const listProcessingOrdersQuery = `
	SELECT
		o.id::text,
		r.external_id,
		r.order_endpoint,
		o.menu_snapshot_version
	FROM orders o
	JOIN restaurants r ON r.id = o.restaurant_id
	WHERE o.status = 'PROCESSING'
	ORDER BY o.created_at, o.id
`

const markOrderAcceptedQuery = `
	UPDATE orders
	SET
		status = 'ACCEPTED',
		external_order_id = $2,
		rejection_reason = NULL,
		updated_at = now()
	WHERE id = $1
		AND status = 'PROCESSING'
	RETURNING id::text
`

const markOrderRejectedQuery = `
	UPDATE orders
	SET
		status = 'REJECTED',
		external_order_id = NULL,
		rejection_reason = $2,
		updated_at = now()
	WHERE id = $1
		AND status = 'PROCESSING'
	RETURNING id::text
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

func (r *Repository) GetOrder(ctx context.Context, orderID string) (orderservice.Order, error) {
	order, err := loadOrderByID(ctx, r.database, orderID)
	if err != nil {
		return orderservice.Order{}, err
	}
	return order, nil
}

func (r *Repository) ListProcessingOrders(ctx context.Context) ([]orderservice.DeliveryOrder, error) {
	rows, err := r.database.Query(ctx, listProcessingOrdersQuery)
	if err != nil {
		return nil, fmt.Errorf("query processing orders: %w", err)
	}
	defer rows.Close()

	orders := make([]orderservice.DeliveryOrder, 0)
	for rows.Next() {
		var order orderservice.DeliveryOrder
		if err := rows.Scan(
			&order.ID,
			&order.RestaurantExternalID,
			&order.OrderEndpoint,
			&order.MenuSnapshotVersion,
		); err != nil {
			return nil, fmt.Errorf("scan processing order: %w", err)
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate processing orders: %w", err)
	}

	for index := range orders {
		items, err := loadDeliveryItems(ctx, r.database, orders[index].ID)
		if err != nil {
			return nil, err
		}
		orders[index].Items = items
	}

	return orders, nil
}

func (r *Repository) MarkAccepted(ctx context.Context, orderID, externalOrderID string) error {
	var updatedID string
	if err := r.database.QueryRow(ctx, markOrderAcceptedQuery, orderID, externalOrderID).Scan(&updatedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("mark order accepted: %w", err)
	}
	return nil
}

func (r *Repository) MarkRejected(
	ctx context.Context,
	orderID string,
	reason orderservice.RejectionReason,
) error {
	var updatedID string
	if err := r.database.QueryRow(ctx, markOrderRejectedQuery, orderID, reason).Scan(&updatedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("mark order rejected: %w", err)
	}
	return nil
}

func (r *Repository) CreateOrder(
	ctx context.Context,
	command orderservice.CreateOrderCommand,
) (orderservice.Order, error) {
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return orderservice.Order{}, fmt.Errorf("begin create order transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	claimed, err := claimIdempotencyKey(ctx, tx, command.IdempotencyKey, command.RequestHash)
	if err != nil {
		return orderservice.Order{}, err
	}
	if !claimed {
		existing, err := loadIdempotentOrder(ctx, tx, command.IdempotencyKey, command.RequestHash)
		if err != nil {
			return orderservice.Order{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return orderservice.Order{}, fmt.Errorf("commit repeated create order transaction: %w", err)
		}
		return existing, nil
	}

	restaurant, err := loadRestaurantState(ctx, tx, command.RestaurantID)
	if err != nil {
		return orderservice.Order{}, err
	}

	menu, err := loadMenuState(ctx, tx, command.RestaurantID, command.Items)
	if err != nil {
		return orderservice.Order{}, err
	}

	created, err := orderservice.BuildProcessingOrder(command, restaurant, menu)
	if err != nil {
		return orderservice.Order{}, err
	}

	if err := tx.QueryRow(
		ctx,
		insertOrderQuery,
		created.UserID,
		created.RestaurantID,
		created.MenuSnapshotVersion,
		created.Status,
		created.TotalPrice,
	).Scan(&created.ID, &created.CreatedAt, &created.UpdatedAt); err != nil {
		return orderservice.Order{}, fmt.Errorf("insert order: %w", err)
	}

	for _, item := range created.Items {
		if _, err := tx.Exec(
			ctx,
			insertOrderItemQuery,
			created.ID,
			item.MenuItemID,
			item.ExternalMenuItemID,
			item.Name,
			item.UnitPrice,
			item.Quantity,
		); err != nil {
			return orderservice.Order{}, fmt.Errorf("insert order item %s: %w", item.MenuItemID, err)
		}
	}

	tag, err := tx.Exec(
		ctx,
		bindIdempotencyKeyQuery,
		createOrderOperation,
		command.IdempotencyKey,
		created.ID,
	)
	if err != nil {
		return orderservice.Order{}, fmt.Errorf("bind idempotency key to order: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return orderservice.Order{}, errors.New("bind idempotency key to order: key disappeared during transaction")
	}

	if err := tx.Commit(ctx); err != nil {
		return orderservice.Order{}, fmt.Errorf("commit create order transaction: %w", err)
	}
	return created, nil
}

func claimIdempotencyKey(
	ctx context.Context,
	tx pgx.Tx,
	key string,
	requestHash string,
) (bool, error) {
	tag, err := tx.Exec(ctx, claimIdempotencyKeyQuery, createOrderOperation, key, requestHash)
	if err != nil {
		return false, fmt.Errorf("claim idempotency key: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func loadIdempotentOrder(
	ctx context.Context,
	tx pgx.Tx,
	key string,
	requestHash string,
) (orderservice.Order, error) {
	var (
		storedHash string
		orderID    string
	)
	if err := tx.QueryRow(
		ctx,
		getIdempotencyKeyQuery,
		createOrderOperation,
		key,
	).Scan(&storedHash, &orderID); err != nil {
		return orderservice.Order{}, fmt.Errorf("read existing idempotency key: %w", err)
	}

	if storedHash != requestHash {
		return orderservice.Order{}, orderservice.ErrIdempotencyKeyReused
	}
	if orderID == "" {
		return orderservice.Order{}, errors.New("idempotency key is not bound to an order")
	}

	order, err := loadOrderByID(ctx, tx, orderID)
	if err != nil {
		return orderservice.Order{}, fmt.Errorf("load idempotent order: %w", err)
	}
	return order, nil
}

func loadRestaurantState(
	ctx context.Context,
	tx pgx.Tx,
	restaurantID string,
) (orderservice.RestaurantState, error) {
	var restaurant orderservice.RestaurantState
	if err := tx.QueryRow(ctx, lockRestaurantQuery, restaurantID).Scan(
		&restaurant.Timezone,
		&restaurant.IsAcceptingOrders,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return orderservice.RestaurantState{}, orderservice.ErrRestaurantNotFound
		}
		return orderservice.RestaurantState{}, fmt.Errorf("lock restaurant: %w", err)
	}

	rows, err := tx.Query(ctx, listWorkingHoursQuery, restaurantID)
	if err != nil {
		return orderservice.RestaurantState{}, fmt.Errorf("query restaurant working hours: %w", err)
	}
	defer rows.Close()

	restaurant.WorkingHours = make([]orderservice.WorkingHours, 0, 7)
	for rows.Next() {
		var (
			dayOfWeek int16
			mode      string
			opensAt   pgtype.Time
			closesAt  pgtype.Time
		)
		if err := rows.Scan(&dayOfWeek, &mode, &opensAt, &closesAt); err != nil {
			return orderservice.RestaurantState{}, fmt.Errorf("scan restaurant working hours: %w", err)
		}

		restaurant.WorkingHours = append(restaurant.WorkingHours, orderservice.WorkingHours{
			DayOfWeek: int(dayOfWeek),
			Mode:      orderservice.WorkingHoursMode(mode),
			OpensAt:   postgresTimeToDuration(opensAt),
			ClosesAt:  postgresTimeToDuration(closesAt),
		})
	}
	if err := rows.Err(); err != nil {
		return orderservice.RestaurantState{}, fmt.Errorf("iterate restaurant working hours: %w", err)
	}

	return restaurant, nil
}

func loadMenuState(
	ctx context.Context,
	tx pgx.Tx,
	restaurantID string,
	requestedItems []orderservice.CreateOrderItemInput,
) (orderservice.MenuState, error) {
	var (
		menuID string
		menu   orderservice.MenuState
	)
	if err := tx.QueryRow(ctx, lockMenuQuery, restaurantID).Scan(
		&menuID,
		&menu.SnapshotVersion,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return orderservice.MenuState{}, fmt.Errorf("restaurant %s has no menu", restaurantID)
		}
		return orderservice.MenuState{}, fmt.Errorf("lock menu: %w", err)
	}

	items, err := loadRequestedMenuItems(ctx, tx, menuID, requestedItems)
	if err != nil {
		return orderservice.MenuState{}, err
	}
	menu.Items = items
	return menu, nil
}

func loadRequestedMenuItems(
	ctx context.Context,
	tx pgx.Tx,
	menuID string,
	requestedItems []orderservice.CreateOrderItemInput,
) ([]orderservice.MenuItemState, error) {
	placeholders := make([]string, 0, len(requestedItems))
	args := make([]any, 0, len(requestedItems)+1)
	args = append(args, menuID)
	for index, item := range requestedItems {
		placeholders = append(placeholders, fmt.Sprintf("$%d", index+2))
		args = append(args, item.MenuItemID)
	}

	query := `
		SELECT
			id::text,
			external_id,
			name,
			price,
			available_quantity,
			is_active
		FROM menu_items
		WHERE menu_id = $1
			AND id IN (` + strings.Join(placeholders, ", ") + `)
	`

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query requested menu items: %w", err)
	}
	defer rows.Close()

	items := make([]orderservice.MenuItemState, 0, len(requestedItems))
	for rows.Next() {
		var item orderservice.MenuItemState
		if err := rows.Scan(
			&item.ID,
			&item.ExternalID,
			&item.Name,
			&item.Price,
			&item.AvailableQuantity,
			&item.IsActive,
		); err != nil {
			return nil, fmt.Errorf("scan requested menu item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate requested menu items: %w", err)
	}
	return items, nil
}

func loadOrderByID(
	ctx context.Context,
	queryer queryer,
	orderID string,
) (orderservice.Order, error) {
	var (
		order           orderservice.Order
		externalOrderID pgtype.Text
		rejectionReason pgtype.Text
	)
	if err := queryer.QueryRow(ctx, getOrderQuery, orderID).Scan(
		&order.ID,
		&order.UserID,
		&order.RestaurantID,
		&externalOrderID,
		&order.MenuSnapshotVersion,
		&order.Status,
		&rejectionReason,
		&order.TotalPrice,
		&order.CreatedAt,
		&order.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return orderservice.Order{}, orderservice.ErrOrderNotFound
		}
		return orderservice.Order{}, fmt.Errorf("query order: %w", err)
	}
	if externalOrderID.Valid {
		value := externalOrderID.String
		order.ExternalOrderID = &value
	}
	if rejectionReason.Valid {
		value := orderservice.RejectionReason(rejectionReason.String)
		order.RejectionReason = &value
	}

	items, err := loadOrderItems(ctx, queryer, orderID)
	if err != nil {
		return orderservice.Order{}, err
	}
	order.Items = items
	return order, nil
}

func loadOrderItems(
	ctx context.Context,
	queryer queryer,
	orderID string,
) ([]orderservice.Item, error) {
	rows, err := queryer.Query(ctx, listOrderItemsQuery, orderID)
	if err != nil {
		return nil, fmt.Errorf("query order items: %w", err)
	}
	defer rows.Close()

	items := make([]orderservice.Item, 0)
	for rows.Next() {
		var item orderservice.Item
		if err := rows.Scan(
			&item.MenuItemID,
			&item.ExternalMenuItemID,
			&item.Name,
			&item.UnitPrice,
			&item.Quantity,
		); err != nil {
			return nil, fmt.Errorf("scan order item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate order items: %w", err)
	}
	return items, nil
}

func loadDeliveryItems(
	ctx context.Context,
	queryer queryer,
	orderID string,
) ([]orderservice.DeliveryItem, error) {
	rows, err := queryer.Query(ctx, listOrderItemsQuery, orderID)
	if err != nil {
		return nil, fmt.Errorf("query delivery order items: %w", err)
	}
	defer rows.Close()

	items := make([]orderservice.DeliveryItem, 0)
	for rows.Next() {
		var (
			menuItemID         string
			externalMenuItemID string
			name               string
			unitPrice          int64
			quantity           int
		)
		if err := rows.Scan(
			&menuItemID,
			&externalMenuItemID,
			&name,
			&unitPrice,
			&quantity,
		); err != nil {
			return nil, fmt.Errorf("scan delivery order item: %w", err)
		}
		items = append(items, orderservice.DeliveryItem{
			ExternalMenuItemID: externalMenuItemID,
			Quantity:           quantity,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate delivery order items: %w", err)
	}
	return items, nil
}

func postgresTimeToDuration(value pgtype.Time) time.Duration {
	if !value.Valid {
		return 0
	}
	return time.Duration(value.Microseconds) * time.Microsecond
}

var _ orderservice.Repository = (*Repository)(nil)
var _ database = (*pgxpool.Pool)(nil)
var _ database = (pgx.Tx)(nil)
