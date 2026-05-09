CREATE TABLE albums (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    artist_id   UUID REFERENCES artists(id),
    release_date DATE,
    cover_image_url TEXT,
    created_at  TIMESTAMPTZ DEFAULT now()