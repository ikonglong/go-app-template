-- name: AddAccount :one
INSERT INTO account (
    id, name, email, phone, avatar_url,
    password_hash, email_verified,
    provider, provider_user_id,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: UpdateAccount :execrows
UPDATE account SET
    name = $2,
    email = $3,
    phone = $4,
    avatar_url = $5,
    password_hash = $6,
    email_verified = $7,
    provider = $8,
    provider_user_id = $9,
    created_at = $10,
    updated_at = $11
WHERE id = $1;

-- name: DeleteAccount :execrows
DELETE FROM account WHERE id = $1;

-- name: GetAccount :one
SELECT * FROM account WHERE id = $1 LIMIT 1;

-- name: GetAccountByEmail :one
SELECT * FROM account WHERE email = $1 LIMIT 1;

-- name: GetAccountByPhone :one
SELECT * FROM account WHERE phone = $1 LIMIT 1;

-- name: GetAccountByProvider :one
SELECT * FROM account
WHERE provider = $1 AND provider_user_id = $2
LIMIT 1;
