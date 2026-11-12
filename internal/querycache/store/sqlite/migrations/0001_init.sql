-- The query cache's store, as first made (docs/designs/query-cache.md#the-store).
-- Times are Unix milliseconds.

-- One per key: which physical entry answers it now.
CREATE TABLE query_cache_logical_entries (
  key         TEXT PRIMARY KEY,
  physical_id INTEGER NOT NULL
);

-- One per write of a key's result, its file in canal-lake named by its
-- namespace and id. Pending while cached_at is NULL, and then expires_at is its
-- lease.
CREATE TABLE query_cache_physical_entries (
  id         INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: the greater is the newer
  namespace  TEXT NOT NULL DEFAULT '',
  key        TEXT NOT NULL,
  cached_at  INTEGER,
  expires_at INTEGER NOT NULL,
  row_count  INTEGER NOT NULL DEFAULT 0,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  columns    BLOB,
  aql        TEXT NOT NULL DEFAULT '',
  sql        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX query_cache_physical_by_key ON query_cache_physical_entries (key);
CREATE INDEX query_cache_physical_by_expiry ON query_cache_physical_entries (expires_at);

-- A key's tags, what a clean selects entries by.
CREATE TABLE query_cache_tags (
  key   TEXT NOT NULL,
  kind  TEXT NOT NULL,
  value TEXT NOT NULL,
  PRIMARY KEY (key, kind, value)
);
CREATE INDEX query_cache_tags_by_tag ON query_cache_tags (kind, value);

-- One row: the schema version an anfra must be at, at least, to use this store.
-- Applying a breaking migration sets it to the migration's own number.
CREATE TABLE query_cache_meta (
  min_store_version INTEGER NOT NULL
);
INSERT INTO query_cache_meta (min_store_version) VALUES (1);
