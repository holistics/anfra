-- The store's queries, which sqlc generates gen/ from (sqlc.yaml). Times are Unix
-- milliseconds. A physical entry is pending while cached_at is NULL, and its
-- expires_at is then its lease (docs/designs/query-cache.md#a-writes-lease).

-- name: Lookup :one
SELECT p.id, p.namespace, p.key, p.cached_at, p.expires_at, p.row_count, p.size_bytes, p.columns, p.aql, p.sql
FROM query_cache_logical_entries l
JOIN query_cache_physical_entries p ON p.id = l.physical_id
WHERE l.key = sqlc.arg(key) AND p.cached_at IS NOT NULL AND p.expires_at > sqlc.arg(now);

-- name: Get :one
SELECT p.id, p.namespace, p.key, p.cached_at, p.expires_at, p.row_count, p.size_bytes, p.columns, p.aql, p.sql
FROM query_cache_physical_entries p
JOIN query_cache_logical_entries l ON l.physical_id = p.id
WHERE p.id = sqlc.arg(id) AND p.cached_at IS NOT NULL AND p.expires_at > sqlc.arg(now);

-- name: BeginWrite :one
INSERT INTO query_cache_physical_entries (namespace, key, expires_at, aql, sql)
VALUES (?, ?, ?, ?, ?) RETURNING id;

-- name: TagKey :exec
INSERT OR IGNORE INTO query_cache_tags (key, kind, value) VALUES (?, ?, ?);

-- name: PendingLease :one
SELECT expires_at FROM query_cache_physical_entries
WHERE id = ? AND key = ? AND cached_at IS NULL;

-- name: CurrentOf :one
SELECT physical_id FROM query_cache_logical_entries WHERE key = ?;

-- name: FillWrite :exec
UPDATE query_cache_physical_entries
SET cached_at = ?, expires_at = ?, row_count = ?, size_bytes = ?, columns = ?
WHERE id = ?;

-- name: MakeCurrent :exec
INSERT INTO query_cache_logical_entries (key, physical_id) VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET physical_id = excluded.physical_id;

-- name: ExpireReplaced :exec
UPDATE query_cache_physical_entries SET expires_at = sqlc.arg(at)
WHERE key = sqlc.arg(key) AND id != sqlc.arg(id) AND cached_at IS NOT NULL AND expires_at > sqlc.arg(at);

-- name: Abandon :execrows
UPDATE query_cache_physical_entries SET expires_at = min(expires_at, CAST(sqlc.arg(now) AS INTEGER))
WHERE id = sqlc.arg(id) AND cached_at IS NULL;

-- name: Sweepable :many
SELECT id, namespace, key, cached_at, expires_at, row_count, size_bytes, columns, aql, sql
FROM query_cache_physical_entries
WHERE expires_at < sqlc.arg(cutoff) ORDER BY expires_at, id LIMIT sqlc.arg(max_entries);

-- name: KeysOf :many
SELECT DISTINCT key FROM query_cache_physical_entries
WHERE id IN (sqlc.slice(ids));

-- name: DeleteEntries :exec
DELETE FROM query_cache_physical_entries WHERE id IN (sqlc.slice(ids));

-- name: DeleteLogical :exec
DELETE FROM query_cache_logical_entries WHERE physical_id IN (sqlc.slice(ids));

-- name: DeleteTagsOfEmptyKey :exec
DELETE FROM query_cache_tags WHERE query_cache_tags.key = sqlc.arg(key) AND NOT EXISTS (
  SELECT 1 FROM query_cache_physical_entries p WHERE p.key = sqlc.arg(key));

-- name: CountAllToExpire :one
SELECT count(*) AS entries, CAST(coalesce(sum(size_bytes), 0) AS INTEGER) AS bytes FROM query_cache_physical_entries
WHERE cached_at IS NOT NULL AND expires_at > sqlc.arg(now);

-- name: ExpireAll :exec
UPDATE query_cache_physical_entries SET expires_at = sqlc.arg(now)
WHERE expires_at > sqlc.arg(now);

-- name: CountTaggedToExpire :one
SELECT count(*) AS entries, CAST(coalesce(sum(p.size_bytes), 0) AS INTEGER) AS bytes FROM query_cache_physical_entries p
WHERE p.cached_at IS NOT NULL AND p.expires_at > sqlc.arg(now) AND p.key IN (
  SELECT t.key FROM query_cache_tags t
  WHERE (t.kind, t.value) IN (
    SELECT json_extract(j.value, '$[0]'), json_extract(j.value, '$[1]') FROM json_each(CAST(sqlc.arg(tags) AS TEXT)) j)
  GROUP BY t.key HAVING count(*) = CAST(sqlc.arg(tag_count) AS INTEGER));

-- name: ExpireTagged :exec
UPDATE query_cache_physical_entries SET expires_at = sqlc.arg(now)
WHERE query_cache_physical_entries.expires_at > sqlc.arg(now) AND query_cache_physical_entries.key IN (
  SELECT t.key FROM query_cache_tags t
  WHERE (t.kind, t.value) IN (
    SELECT json_extract(j.value, '$[0]'), json_extract(j.value, '$[1]') FROM json_each(CAST(sqlc.arg(tags) AS TEXT)) j)
  GROUP BY t.key HAVING count(*) = CAST(sqlc.arg(tag_count) AS INTEGER));
