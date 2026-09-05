CREATE TABLE tracks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title        TEXT NOT NULL,
    artist_id    UUID NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    album_id     UUID REFERENCES albums(id) ON DELETE SET NULL,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    bitrate      INTEGER NOT NULL DEFAULT 0,
    format       TEXT NOT NULL DEFAULT '',
    file_size    BIGINT NOT NULL DEFAULT 0,
    track_number INTEGER NOT NULL DEFAULT 0,
    disc_number  INTEGER NOT NULL DEFAULT 0,
    storage_key  TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX tracks_artist_id_idx ON tracks (artist_id);
CREATE INDEX tracks_album_id_idx ON tracks (album_id);
