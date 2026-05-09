CREATE TABLE artists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    bio         TEXT,
    created_at  TIMESTAMPTZ DEFAULT now()
);