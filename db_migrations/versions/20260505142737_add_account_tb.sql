-- +goose Up
CREATE TABLE IF NOT EXISTS account (
    id VARCHAR(30) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(255) UNIQUE,
    phone VARCHAR(20) UNIQUE,
    avatar_url VARCHAR(500),

    password_hash VARCHAR(128),
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,

    provider VARCHAR(20),
    provider_user_id VARCHAR(100),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (provider, provider_user_id),
    CHECK (
        password_hash IS NOT NULL
        OR (provider IS NOT NULL AND provider_user_id IS NOT NULL)
    ),
    CHECK (
        (provider IS NULL) = (provider_user_id IS NULL)
    )
);

-- +goose Down
DROP TABLE IF EXISTS account;
