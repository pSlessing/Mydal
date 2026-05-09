CREATE TABLE tracks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    artist_id   UUID REFERENCES artists(id),
    album_id    UUID REFERENCES albums(id),
    duration_ms INTEGER NOT NULL,
    storage_key TEXT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT now()
);