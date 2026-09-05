CREATE TABLE albums (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title        TEXT NOT NULL,
    artist_id    UUID NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    release_date DATE,
    cover_key    TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX albums_artist_id_idx ON albums (artist_id);
