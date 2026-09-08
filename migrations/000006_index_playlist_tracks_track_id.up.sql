-- tracks(id) ON DELETE CASCADE fires against playlist_tracks by track_id, and
-- the only index covering that column was (playlist_id, track_id) - unusable
-- for a lookup keyed on track_id alone. Every track delete scanned the whole
-- table to find the rows the cascade was about to remove.
CREATE INDEX playlist_tracks_track_id_idx ON playlist_tracks (track_id);
