# The query cache store's migrations

Each file is one version of the store's schema, applied in order: `NNNN_what.sql`.
A store is made by applying them all, and an older one is brought up to date by
applying those after its version (SQLite's `user_version`). sqlc reads them as
the schema it checks `queries.sql` against: run `scripts/sqlc.sh generate` after
adding one.

- **Add, don't edit.** A migration that has shipped is never changed: a store may
  already have it.
- **Additive by default:** a column with a default, a table, an index. An older
  anfra still using the store keeps working.
- **A breaking migration says so on its first line, `-- breaking`:** one that drops
  or renames, or empties tables whose rows can't follow it. The store then
  requires an anfra at that migration's number or later (`min_store_version`).
  For example, after `0004_drop_x.sql`, marked breaking, an anfra whose last
  migration is 3 stops using the store rather than misread it. An additive
  migration changes nothing for older anfras
  (docs/designs/query-cache.md#its-schema-migrations).
