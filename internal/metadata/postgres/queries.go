package postgres

// The catalog queries preserve source types and pair composite foreign keys by position.
const tablesSQL = `
SELECT
  current_database()                 AS database_name,
  n.nspname                          AS schema_name,
  c.relname                          AS table_name,
  obj_description(c.oid, 'pg_class') AS description
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, c.relname
`

const columnsSQL = `
SELECT
  current_database()                   AS database_name,
  n.nspname                            AS schema_name,
  c.relname                            AS table_name,
  a.attname                            AS column_name,
  format_type(a.atttypid, a.atttypmod) AS data_type,
  col_description(c.oid, a.attnum)     AS description
FROM pg_catalog.pg_attribute a
JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE a.attnum > 0
  AND NOT a.attisdropped
  AND c.relkind IN ('r', 'p')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, c.relname, a.attnum
`

const primaryKeysSQL = `
SELECT
  current_database() AS database_name,
  n.nspname          AS schema_name,
  c.relname          AS table_name,
  a.attname          AS column_name
FROM pg_catalog.pg_constraint con
JOIN pg_catalog.pg_class c ON c.oid = con.conrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ANY (con.conkey)
WHERE con.contype = 'p'
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
`

const foreignKeysSQL = `
SELECT
  current_database() AS database_name,
  con.conname        AS constraint_name,
  sn.nspname         AS schema_name,
  sc.relname         AS table_name,
  sa.attname         AS column_name,
  tn.nspname         AS ref_schema_name,
  tc.relname         AS ref_table_name,
  ta.attname         AS ref_column_name
FROM pg_catalog.pg_constraint con
CROSS JOIN LATERAL unnest(con.conkey, con.confkey) WITH ORDINALITY AS pair(attnum, ref_attnum, ord)
JOIN pg_catalog.pg_class sc     ON sc.oid = con.conrelid
JOIN pg_catalog.pg_namespace sn ON sn.oid = sc.relnamespace
JOIN pg_catalog.pg_class tc     ON tc.oid = con.confrelid
JOIN pg_catalog.pg_namespace tn ON tn.oid = tc.relnamespace
JOIN pg_catalog.pg_attribute sa ON sa.attrelid = con.conrelid  AND sa.attnum = pair.attnum
JOIN pg_catalog.pg_attribute ta ON ta.attrelid = con.confrelid AND ta.attnum = pair.ref_attnum
WHERE con.contype = 'f'
  AND sn.nspname NOT IN ('pg_catalog', 'information_schema')
  AND tn.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY sn.nspname, sc.relname, con.conname, pair.ord
`
