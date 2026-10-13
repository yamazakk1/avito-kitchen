package restaurant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	restaurantservice "github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/service/restaurant"
)

const listRestaurantsQuery = `
	SELECT
		r.id,
		r.external_id,
		r.name,
		r.order_endpoint,
		r.timezone,
		r.is_accepting_orders,
		r.created_at,
		r.updated_at,
		h.day_of_week,
		h.mode,
		h.opens_at,
		h.closes_at
	FROM restaurants r
	LEFT JOIN restaurant_working_hours h ON h.restaurant_id = r.id
	ORDER BY r.created_at, r.id, h.day_of_week
`

const createRestaurantQuery = `
	INSERT INTO restaurants (
		id,
		external_id,
		name,
		order_endpoint,
		timezone,
		is_accepting_orders
	)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING created_at, updated_at
`

const createWorkingHoursQuery = `
	INSERT INTO restaurant_working_hours (
		restaurant_id,
		day_of_week,
		mode,
		opens_at,
		closes_at
	)
	VALUES ($1, $2, $3, $4, $5)
`
const updateRestaurantQuery = `
	UPDATE restaurants
	SET
		name = COALESCE($2, name),
		order_endpoint = COALESCE($3, order_endpoint),
		timezone = COALESCE($4, timezone),
		is_accepting_orders = COALESCE($5, is_accepting_orders),
		updated_at = now()
	WHERE id = $1
	RETURNING
		id,
		external_id,
		name,
		order_endpoint,
		timezone,
		is_accepting_orders,
		created_at,
		updated_at
`

const deleteWorkingHoursQuery = `
	DELETE FROM restaurant_working_hours
	WHERE restaurant_id = $1
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

const createMenuQuery = `
	INSERT INTO menus (
		id,
		restaurant_id
	)
	VALUES (gen_random_uuid(), $1)
`

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
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

func (r *Repository) ListRestaurants(ctx context.Context) ([]restaurantservice.Restaurant, error) {
	rows, err := r.database.Query(ctx, listRestaurantsQuery)
	if err != nil {
		return nil, fmt.Errorf("query restaurants: %w", err)
	}
	defer rows.Close()

	restaurants := make([]restaurantservice.Restaurant, 0)
	for rows.Next() {
		var (
			id                string
			externalID        string
			name              string
			orderEndpoint     string
			timezone          string
			isAcceptingOrders bool
			createdAt         time.Time
			updatedAt         time.Time
			dayOfWeek         pgtype.Int2
			mode              pgtype.Text
			opensAt           pgtype.Time
			closesAt          pgtype.Time
		)

		if err := rows.Scan(
			&id,
			&externalID,
			&name,
			&orderEndpoint,
			&timezone,
			&isAcceptingOrders,
			&createdAt,
			&updatedAt,
			&dayOfWeek,
			&mode,
			&opensAt,
			&closesAt,
		); err != nil {
			return nil, fmt.Errorf("scan restaurant: %w", err)
		}

		if len(restaurants) == 0 || restaurants[len(restaurants)-1].ID != id {
			restaurants = append(restaurants, restaurantservice.Restaurant{
				ID:                id,
				ExternalID:        externalID,
				Name:              name,
				OrderEndpoint:     orderEndpoint,
				Timezone:          timezone,
				IsAcceptingOrders: isAcceptingOrders,
				WorkingHours:      make([]restaurantservice.WorkingHours, 0, 7),
				CreatedAt:         createdAt,
				UpdatedAt:         updatedAt,
			})
		}

		if !dayOfWeek.Valid {
			continue
		}
		if !mode.Valid {
			return nil, fmt.Errorf("restaurant %s has working hours without mode", id)
		}

		restaurant := &restaurants[len(restaurants)-1]
		restaurant.WorkingHours = append(restaurant.WorkingHours, restaurantservice.WorkingHours{
			DayOfWeek: int(dayOfWeek.Int16),
			Mode:      restaurantservice.WorkingHoursMode(mode.String),
			OpensAt:   postgresTimeToDuration(opensAt),
			ClosesAt:  postgresTimeToDuration(closesAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate restaurants: %w", err)
	}

	return restaurants, nil
}

func (r *Repository) CreateRestaurant(
	ctx context.Context,
	restaurant restaurantservice.Restaurant,
) (restaurantservice.Restaurant, error) {
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return restaurantservice.Restaurant{}, fmt.Errorf(
			"begin create restaurant transaction: %w",
			err,
		)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	created := restaurant

	err = tx.QueryRow(
		ctx,
		createRestaurantQuery,
		restaurant.ID,
		restaurant.ExternalID,
		restaurant.Name,
		restaurant.OrderEndpoint,
		restaurant.Timezone,
		restaurant.IsAcceptingOrders,
	).Scan(&created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		if isExternalIDConflict(err) {
			return restaurantservice.Restaurant{},
				restaurantservice.ErrRestaurantAlreadyExists
		}

		return restaurantservice.Restaurant{},
			fmt.Errorf("insert restaurant: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		createMenuQuery,
		restaurant.ID,
	); err != nil {
		return restaurantservice.Restaurant{},
			fmt.Errorf("insert initial menu: %w", err)
	}

	for _, hours := range restaurant.WorkingHours {
		var opensAt any
		var closesAt any

		if hours.Mode == restaurantservice.WorkingHoursInterval {
			opensAt = durationToPostgresTime(hours.OpensAt)
			closesAt = durationToPostgresTime(hours.ClosesAt)
		}

		if _, err := tx.Exec(
			ctx,
			createWorkingHoursQuery,
			restaurant.ID,
			hours.DayOfWeek,
			hours.Mode,
			opensAt,
			closesAt,
		); err != nil {
			return restaurantservice.Restaurant{},
				fmt.Errorf(
					"insert working hours for day %d: %w",
					hours.DayOfWeek,
					err,
				)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return restaurantservice.Restaurant{},
			fmt.Errorf("commit create restaurant transaction: %w", err)
	}

	return created, nil
}

func (r *Repository) UpdateRestaurant(
	ctx context.Context,
	restaurantID string,
	update restaurantservice.RestaurantUpdate,
) (restaurantservice.Restaurant, error) {
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return restaurantservice.Restaurant{}, fmt.Errorf(
			"begin update restaurant transaction: %w",
			err,
		)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		name              any
		orderEndpoint     any
		timezone          any
		isAcceptingOrders any
	)
	if update.Name != nil {
		name = *update.Name
	}
	if update.OrderEndpoint != nil {
		orderEndpoint = *update.OrderEndpoint
	}
	if update.Timezone != nil {
		timezone = *update.Timezone
	}
	if update.IsAcceptingOrders != nil {
		isAcceptingOrders = *update.IsAcceptingOrders
	}

	var updated restaurantservice.Restaurant
	err = tx.QueryRow(
		ctx,
		updateRestaurantQuery,
		restaurantID,
		name,
		orderEndpoint,
		timezone,
		isAcceptingOrders,
	).Scan(
		&updated.ID,
		&updated.ExternalID,
		&updated.Name,
		&updated.OrderEndpoint,
		&updated.Timezone,
		&updated.IsAcceptingOrders,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return restaurantservice.Restaurant{}, restaurantservice.ErrRestaurantNotFound
		}
		return restaurantservice.Restaurant{}, fmt.Errorf("update restaurant row: %w", err)
	}

	if update.WorkingHours != nil {
		if _, err := tx.Exec(ctx, deleteWorkingHoursQuery, restaurantID); err != nil {
			return restaurantservice.Restaurant{}, fmt.Errorf("delete old working hours: %w", err)
		}

		for _, hours := range *update.WorkingHours {
			var opensAt any
			var closesAt any
			if hours.Mode == restaurantservice.WorkingHoursInterval {
				opensAt = durationToPostgresTime(hours.OpensAt)
				closesAt = durationToPostgresTime(hours.ClosesAt)
			}

			if _, err := tx.Exec(
				ctx,
				createWorkingHoursQuery,
				restaurantID,
				hours.DayOfWeek,
				hours.Mode,
				opensAt,
				closesAt,
			); err != nil {
				return restaurantservice.Restaurant{}, fmt.Errorf(
					"insert updated working hours for day %d: %w",
					hours.DayOfWeek,
					err,
				)
			}
		}
	}

	updated.WorkingHours, err = loadWorkingHours(ctx, tx, restaurantID)
	if err != nil {
		return restaurantservice.Restaurant{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return restaurantservice.Restaurant{}, fmt.Errorf(
			"commit update restaurant transaction: %w",
			err,
		)
	}

	return updated, nil
}

func loadWorkingHours(
	ctx context.Context,
	queryer queryer,
	restaurantID string,
) ([]restaurantservice.WorkingHours, error) {
	rows, err := queryer.Query(ctx, listWorkingHoursQuery, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("query working hours: %w", err)
	}
	defer rows.Close()

	workingHours := make([]restaurantservice.WorkingHours, 0, 7)
	for rows.Next() {
		var (
			dayOfWeek int16
			mode      string
			opensAt   pgtype.Time
			closesAt  pgtype.Time
		)
		if err := rows.Scan(&dayOfWeek, &mode, &opensAt, &closesAt); err != nil {
			return nil, fmt.Errorf("scan working hours: %w", err)
		}

		workingHours = append(workingHours, restaurantservice.WorkingHours{
			DayOfWeek: int(dayOfWeek),
			Mode:      restaurantservice.WorkingHoursMode(mode),
			OpensAt:   postgresTimeToDuration(opensAt),
			ClosesAt:  postgresTimeToDuration(closesAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate working hours: %w", err)
	}

	return workingHours, nil
}

func postgresTimeToDuration(value pgtype.Time) time.Duration {
	if !value.Valid {
		return 0
	}
	return time.Duration(value.Microseconds) * time.Microsecond
}

func durationToPostgresTime(value time.Duration) pgtype.Time {
	return pgtype.Time{
		Microseconds: value.Microseconds(),
		Valid:        true,
	}
}

func isExternalIDConflict(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) &&
		postgresError.Code == "23505" &&
		postgresError.ConstraintName == "restaurants_external_id_unique"
}

var _ restaurantservice.Repository = (*Repository)(nil)
var _ database = (*pgxpool.Pool)(nil)
var _ database = (pgx.Tx)(nil)
