BEGIN;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name VARCHAR(50) NOT NULL
        CHECK (length(trim(first_name)) > 0),
    last_name VARCHAR(50)
        CHECK (last_name IS NULL OR length(trim(last_name)) > 0),

    email VARCHAR(255) NOT NULL
        CHECK (
            email = trim(email)
            AND email ~* '^[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}$'
        ),

    password_hash TEXT NOT NULL
        CHECK (length(password_hash) > 0),

    phone_number VARCHAR(16) NOT NULL
        CHECK (
            phone_number ~ '^\+[1-9][0-9]{7,14}$'
        ), 

    status BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    updated_at TIMESTAMPTZ,

    deleted_at TIMESTAMPTZ,

    CONSTRAINT users_updated_at_check
        CHECK (updated_at IS NULL OR updated_at >= created_at),

    CONSTRAINT users_deleted_at_check
        CHECK (deleted_at IS NULL OR deleted_at >= created_at)
);

CREATE UNIQUE INDEX users_email_active_key
    ON users (email)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX users_phone_number_active_key
    ON users (phone_number)
    WHERE deleted_at IS NULL;

COMMIT;