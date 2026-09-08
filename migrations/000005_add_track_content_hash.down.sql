DROP INDEX IF EXISTS tracks_content_hash_key;
ALTER TABLE tracks DROP COLUMN IF EXISTS content_hash;
