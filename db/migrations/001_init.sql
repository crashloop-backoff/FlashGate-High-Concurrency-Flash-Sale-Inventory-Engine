-- +goose Up
-- 001_init.sql
-- Target: PostgreSQL 16
-- Invariant utama: available_stock tidak boleh < 0 atau > total_stock.

CREATE TABLE events (
    id              UUID PRIMARY KEY,
    name            TEXT NOT NULL,
    total_stock     INTEGER NOT NULL CHECK (total_stock > 0),
    available_stock INTEGER NOT NULL CHECK (available_stock >= 0 AND available_stock <= total_stock),
    starts_at       TIMESTAMPTZ NOT NULL,
    ends_at         TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_events_window CHECK (ends_at > starts_at)
);

CREATE TABLE orders (
    id              UUID PRIMARY KEY,
    event_id        UUID NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    user_id         TEXT NOT NULL,
    quantity        INTEGER NOT NULL CHECK (quantity > 0 AND quantity <= 5),
    status          TEXT NOT NULL CHECK (status IN ('RESERVED', 'PAID', 'CANCELLED', 'EXPIRED')),
    idempotency_key VARCHAR(128) NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_orders_idempotency_key UNIQUE (idempotency_key)
);

CREATE INDEX idx_orders_expiry_sweep
    ON orders (expires_at)
    WHERE status = 'RESERVED';

CREATE INDEX idx_orders_event_status
    ON orders (event_id, status);

CREATE INDEX idx_orders_user_event
    ON orders (user_id, event_id);

-- +goose Down
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS events;
