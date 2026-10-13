CREATE TABLE restaurants (
    id UUID PRIMARY KEY,
    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    order_endpoint TEXT NOT NULL,
    timezone TEXT NOT NULL,
    is_accepting_orders BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT restaurants_external_id_unique UNIQUE (external_id)
);

CREATE TABLE restaurant_working_hours (
    restaurant_id UUID NOT NULL REFERENCES restaurants (id),
    day_of_week SMALLINT NOT NULL,
    mode TEXT NOT NULL,
    opens_at TIME,
    closes_at TIME,

    CONSTRAINT restaurant_working_hours_primary_key PRIMARY KEY (restaurant_id, day_of_week),
    CONSTRAINT restaurant_working_hours_day_check CHECK (day_of_week BETWEEN 1 AND 7),
    CONSTRAINT restaurant_working_hours_mode_check CHECK (mode IN ('CLOSED', 'INTERVAL', 'OPEN_24H')),
    CONSTRAINT restaurant_working_hours_interval_check CHECK (
        (
            mode = 'INTERVAL'
            AND opens_at IS NOT NULL
            AND closes_at IS NOT NULL
            AND opens_at < closes_at
        )
        OR
        (
            mode IN ('CLOSED', 'OPEN_24H')
            AND opens_at IS NULL
            AND closes_at IS NULL
        )
    )
);

CREATE TABLE menus (
    id UUID PRIMARY KEY,
    restaurant_id UUID NOT NULL REFERENCES restaurants (id),
    snapshot_version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT menus_restaurant_id_unique UNIQUE (restaurant_id),
    CONSTRAINT menus_snapshot_version_check CHECK (snapshot_version >= 0)
);

CREATE TABLE menu_items (
    id UUID PRIMARY KEY,
    menu_id UUID NOT NULL REFERENCES menus (id),
    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    price BIGINT NOT NULL,
    available_quantity INTEGER NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT menu_items_menu_external_id_unique UNIQUE (menu_id, external_id),
    CONSTRAINT menu_items_price_check CHECK (price >= 0),
    CONSTRAINT menu_items_available_quantity_check CHECK (available_quantity >= 0)
);

CREATE INDEX idx_menu_items_active
    ON menu_items (menu_id)
    WHERE is_active = true;
