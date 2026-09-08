-- The SHA-256 of the uploaded audio, recorded as the file is streamed to the
-- blob store. It is the dedup key for the ingestion pipeline and the source of
-- the ETag on the streaming endpoint.
ALTER TABLE tracks ADD COLUMN content_hash TEXT;

-- Partial, because most rows have no file yet: a track with no upload has no
-- hash, and several of those must be able to coexist.
CREATE UNIQUE INDEX tracks_content_hash_key
    ON tracks (content_hash)
    WHERE content_hash IS NOT NULL;
