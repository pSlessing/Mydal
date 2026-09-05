CREATE TABLE playlists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ordered membership. The position uniqueness is deferrable so a reorder can
-- shuffle rows inside one transaction without tripping over itself.
CREATE TABLE playlist_tracks (
    playlist_id UUID    NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id    UUID    NOT NULL REFERENCES tracks(id)    ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    PRIMARY KEY (playlist_id, track_id),
    UNIQUE (playlist_id, position) DEFERRABLE INITIALLY IMMEDIATE
);
