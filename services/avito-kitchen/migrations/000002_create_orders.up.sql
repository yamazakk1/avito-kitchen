CREATE TABLE orders (
    id UUID PRIMARY KEY,
    user_id TEXT NOT NULL,
    restaurant_id UUID NOT NULL REFERENCES restaurants (id),
    external_order_id TEXT,
    menu_snapshot_version BIGINT NOT NULL,
    status TEXT NOT NULL,
    rejection_reason TEXT,
    total_price BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT orders_user_id_check CHECK (length(user_id) > 0),
    CONSTRAINT orders_menu_snapshot_version_check CHECK (menu_snapshot_version >= 0),
    CONSTRAINT orders_status_check CHECK (status IN ('PROCESSING', 'ACCEPTED', 'REJECTED')),
    CONSTRAINT orders_rejection_reason_check CHECK (
        (
            status = 'REJECTED'
            AND rejection_reason IS NOT NULL
            AND rejection_reason IN (
                'OUT_OF_STOCK',
                'ITEM_UNAVAILABLE',
                'NOT_ACCEPTING_ORDERS',
                'STALE_MENU_SNAPSHOT'
            )
        )
        OR
        (
            status <> 'REJECTED'
            AND rejection_reason IS NULL
        )
    ),
    CONSTRAINT orders_total_price_check CHECK (total_price >= 0)
);

CREATE TABLE order_items (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders (id),
    menu_item_id UUID NOT NULL REFERENCES menu_items (id),
    external_menu_item_id TEXT NOT NULL,
    name TEXT NOT NULL,
    unit_price BIGINT NOT NULL,
    quantity INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT order_items_order_menu_item_unique UNIQUE (order_id, menu_item_id),
    CONSTRAINT order_items_external_menu_item_id_check CHECK (length(external_menu_item_id) > 0),
    CONSTRAINT order_items_name_check CHECK (length(name) > 0),
    CONSTRAINT order_items_unit_price_check CHECK (unit_price >= 0),
    CONSTRAINT order_items_quantity_check CHECK (quantity > 0)
);

CREATE TABLE idempotency_keys (
    operation TEXT NOT NULL,
    key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    order_id UUID UNIQUE REFERENCES orders (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT idempotency_keys_primary_key PRIMARY KEY (operation, key),
    CONSTRAINT idempotency_keys_operation_check CHECK (length(operation) > 0),
    CONSTRAINT idempotency_keys_key_check CHECK (length(key) BETWEEN 1 AND 255),
    CONSTRAINT idempotency_keys_request_hash_check CHECK (length(request_hash) = 64)
);

CREATE INDEX idx_orders_processing_created_at
    ON orders (created_at)
    WHERE status = 'PROCESSING';
