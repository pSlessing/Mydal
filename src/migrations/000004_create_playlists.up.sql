CREATE TABLE playlists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT,
    song_ids    UUID[] NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT now()
);