# LookML derived-table persistence → AML `persistence` and `Schedule`

> **Not in use.** anfra does not persist query models or run schedules yet, so the conversion drops every
> persistence parameter of a derived table (see
> [references/derived-tables.md](../references/derived-tables.md#persistence-parameters)). This page is the
> conversion for Holistics, kept for when anfra supports it. It continues
> [references/derived-tables.md](../references/derived-tables.md): the query model it persists is converted there.

## Persistence strategy

`datagroup_trigger`, `sql_trigger_value`, `interval_trigger`, `persist_for`

Looker: [`datagroup_trigger`](https://docs.cloud.google.com/looker/docs/reference/param-view-datagroup-trigger),
[`sql_trigger_value`](https://docs.cloud.google.com/looker/docs/reference/param-view-sql-trigger-value),
[`interval_trigger`](https://docs.cloud.google.com/looker/docs/reference/param-view-interval-trigger),
[`persist_for`](https://docs.cloud.google.com/looker/docs/reference/param-view-persist-for-for-derived-table) ·
Holistics: [Query Model Persistence](https://docs.holistics.io/docs/persistence),
[AML Persistence](https://docs.holistics.io/reference/aml/persistence)

**Holistics persistence runs on a schedule only.** It has no trigger that reacts to data. Every Looker trigger
becomes a cron line.

1. Any of the four parameters → `persistence: FullPersistence { schema: '<schema>' }` on the query model
   (`IncrementalPersistence` with [`increment_key`](#increment_key-increment_offset)).
2. `schema` is required and is not in the LookML: Looker's scratch schema is a connection setting. Ask the user
   for it. The Holistics connection needs write permission on that schema.
3. Find the trigger:
   - `datagroup_trigger: g` → the `interval_trigger` of datagroup `g`; if it has none, its `sql_trigger`. A
     datagroup with only `max_cache_age` → no schedule; report it (see [model.md](../references/model.md#datagroup)).
   - `sql_trigger_value` → that SQL.
   - `interval_trigger`, `persist_for` → the duration.
   - If several are present, use `datagroup_trigger`, as Looker does.
4. Turn the trigger into a cron line with the table below.
5. Write all schedules in one constant, `const schedules = [ … ]`, in `schedules.aml` at the root of the project.
   Emit one `Schedule` per cron line, listing all models with that cron. Keep the Looker trigger as a comment.

| Trigger | Cron |
|---|---|
| `"N minutes"`, N divides 60 | `0,N,2N,… * * * *`. Below 5 minutes → every 5 minutes, the smallest frequency. |
| `"N hours"`, N divides 24 | `0 0,N,2N,… * * *`; `"24 hours"` → `0 0 * * *` |
| another duration (`"36 hours"`, `"5 hours"`) | the nearest cron; report it |
| SQL that returns the current date (`SELECT CURRENT_DATE`, `CURDATE()`) | `0 0 * * *` |
| SQL that returns the current hour | `0 * * * *` |
| SQL that returns the week, month or year of the current date | `0 0 * * 1`, `0 0 1 * *`, `0 0 1 1 *` |
| SQL that shifts the clock ("daily at 02:00") | read the offset from the SQL: `0 2 * * *` |
| SQL that reads data (`SELECT MAX(id) FROM t`, `SELECT MAX(completed_at) FROM etl_jobs`) | Unsupported. A person must choose the cron. Emit a line with a `// TODO` comment that quotes the SQL. |
| `SELECT COUNT(*) FROM ${upstream.SQL_TABLE_NAME}` ("rebuild after the upstream table") | no schedule of its own when a scheduled model depends on it; otherwise the cron of the upstream model |
| a constant (`SELECT 1`): build once | the `persistence` block and no schedule; report it |

Write cron fields as numbers and explicit lists (`0 0,6,12,18 * * *`, `0 8 * * 1`). Do not use names (`MON`).

**Chains.** A scheduled model also rebuilds the persisted models that it reads. For a chain of derived tables with
one datagroup, schedule only the models that no other persisted model of the group reads. Give a parent its own
schedule and `on_cascade: 'reuse'` only when its Looker trigger was slower than the child's.

```lookml
datagroup: ecommerce_etl {
  sql_trigger: SELECT MAX(completed_at) FROM etl.jobs ;;
  max_cache_age: "24 hours"
}

view: user_order_facts {
  derived_table: {
    sql: … ;;
    datagroup_trigger: ecommerce_etl
  }
}

view: active_users {
  derived_table: {
    sql: SELECT id, email FROM public.users WHERE deleted_at IS NULL ;;
    interval_trigger: "6 hours"
  }

  dimension: id { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: email { type: string  sql: ${TABLE}.email ;; }
}
```
```aml
Model active_users {
  type: 'query'
  label: 'Active Users'
  data_source_name: 'demodb'
  models: []

  query: @sql SELECT id, email FROM public.users WHERE deleted_at IS NULL ;;

  persistence: FullPersistence {
    schema: 'persisted'
  }

  dimension id {
    label: 'Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.id }};;
  }
  dimension email {
    label: 'Email'
    type: 'text'
    definition: @sql {{ #SOURCE.email }};;
  }
}
```

`user_order_facts` gets the same `persistence` block. Then, in `schedules.aml`:

```aml
const schedules = [
  // TODO: set the time. Looker: datagroup ecommerce_etl, sql_trigger: SELECT MAX(completed_at) FROM etl.jobs
  Schedule { cron: '0 5 * * *', models: [user_order_facts] },

  // Looker: interval_trigger "6 hours"
  Schedule { cron: '0 0,6,12,18 * * *', models: [active_users] }
]
```

Differences:

- **Trigger and clock.** "Rebuild when the data changed" cannot be expressed. A cron rebuilds whether or not
  anything changed, and it can run before the load has finished.
- **`persist_for`.** Looker rebuilds the table when a user queries it and it is older than the duration. A cron
  rebuilds it on time, used or not.
- **Failure.** When a build fails, Holistics runs the model's SQL live. Looker keeps serving the previous table.
- **Time zone.** Looker's time-based triggers follow the database time zone. The time zone of Holistics cron lines
  is not documented. Mark for review.
- **Cache.** `max_cache_age` and the cache part of a datagroup are not converted here
  (see [model.md](../references/model.md#persist_for-persist_with)).

## `increment_key`, `increment_offset`

Looker: [`increment_key`](https://docs.cloud.google.com/looker/docs/reference/param-view-increment-key),
[`increment_offset`](https://docs.cloud.google.com/looker/docs/reference/param-view-increment-offset),
[Incremental PDTs](https://docs.cloud.google.com/looker/docs/incremental-pdts) ·
Holistics: [Incremental Persistence](https://docs.holistics.io/docs/persistence#incremental-persistence)

1. `increment_key: "k"` → `persistence: IncrementalPersistence { incremental_column: '<column>' }`. `<column>`
   is the result column behind dimension `k` of the view. For a timeframe (`session_date`), it is the column of
   its dimension group.
2. Remove `{% incrementcondition %} … {% endincrementcondition %}` from the SQL, with the `WHERE` or `AND` that
   it leaves behind. Holistics adds its own filter around the whole query.
3. `primary_key: '<column>'` → the key column of the table, if the view declares one. For a table with one row per
   increment value (daily totals), use the increment column.
4. `increment_offset` → Unsupported. Drop it and report it.
5. The schedule comes from the trigger, as in [Persistence strategy](#persistence-strategy).

```lookml
view: daily_revenue {
  derived_table: {
    sql:
      SELECT created_at::date AS order_date, COUNT(*) AS orders, SUM(amount) AS revenue
      FROM public.orders
      WHERE {% incrementcondition %} created_at {% endincrementcondition %}
      GROUP BY 1 ;;
    sql_trigger_value: SELECT CURRENT_DATE ;;
    increment_key: "order_date"
    increment_offset: 3
  }

  dimension: order_date { primary_key: yes  type: date  sql: ${TABLE}.order_date ;; }
  dimension: revenue { type: number  sql: ${TABLE}.revenue ;; }
}
```
```aml
Model daily_revenue {
  type: 'query'
  label: 'Daily Revenue'
  data_source_name: 'demodb'
  models: []

  query: @sql
    SELECT created_at::date AS order_date, COUNT(*) AS orders, SUM(amount) AS revenue
    FROM public.orders
    GROUP BY 1
  ;;

  persistence: IncrementalPersistence {
    schema: 'persisted'
    incremental_column: 'order_date'
    primary_key: 'order_date'
  }

  dimension order_date {
    label: 'Order Date'
    type: 'date'
    primary_key: true
    definition: @sql {{ #SOURCE.order_date }};;
  }
  dimension revenue {
    label: 'Revenue'
    type: 'number'
    definition: @sql {{ #SOURCE.revenue }};;
  }
}
```

Mark every incremental model for review. The two tools do not use the same method:

| | Looker | Holistics |
|---|---|---|
| Rows rebuilt on each run | from the start of the latest period in the table, minus `increment_offset` periods | only rows with `incremental_column` greater than the current maximum |
| The latest, unfinished period | rebuilt on every run | not visited again: a daily row first written at 10:00 keeps its 10:00 totals |
| Late data in older periods | covered by `increment_offset` | not covered |
| Where the filter applies | inside the SQL, before aggregation | around the finished query, after aggregation |

So incremental persistence is safe only in two cases: the cron runs after each period is complete and the query
excludes the current period; or the increment column is a row-level timestamp that only grows (`updated_at`),
with a real `primary_key`. In every other case, emit `FullPersistence` and report the change.

## `create_process`, `sql_create`

Looker: [`create_process`](https://docs.cloud.google.com/looker/docs/reference/param-view-create-process),
[`sql_create`](https://docs.cloud.google.com/looker/docs/reference/param-view-sql-create) ·
Holistics: [Custom Persistence DDL](https://docs.holistics.io/docs/persistence#custom-persistence-ddl)

Both give Looker the statements that build the table `${SQL_TABLE_NAME}`. A query model always needs a `query`;
`custom_ddl` is a template around it, with the variables `{{ persisted_table }}`, `{{ parsed_query }}` and
`{{ index_name }}`.

| LookML statements | Conversion |
|---|---|
| one `CREATE TABLE ${SQL_TABLE_NAME} AS <select>` | `query` = the `<select>`, with plain `FullPersistence` |
| `CREATE TABLE ${SQL_TABLE_NAME} (…)` + `INSERT INTO ${SQL_TABLE_NAME} <select>` | `query` = the `<select>`; `custom_ddl` = `CREATE TABLE {{ persisted_table }} (…); INSERT INTO {{ persisted_table }} {{ parsed_query }};` |
| more statements on the table (`CREATE INDEX`, `ANALYZE`, `GRANT`, `ALTER TABLE`) | append them to `custom_ddl`; `${SQL_TABLE_NAME}` → `{{ persisted_table }}`, index names → `{{ index_name }}` |
| statements that create something else (a BigQuery ML model, a function, a staging table) | Unsupported. Build it in the database and use a table model on the result. |

```lookml
view: order_totals {
  derived_table: {
    create_process: {
      sql_step:
        CREATE TABLE ${SQL_TABLE_NAME} AS
        SELECT user_id, SUM(amount) AS total_amount
        FROM public.orders
        GROUP BY 1 ;;
      sql_step: CREATE INDEX order_totals_user ON ${SQL_TABLE_NAME} (user_id) ;;
    }
    sql_trigger_value: SELECT CURRENT_DATE ;;
  }

  dimension: user_id { primary_key: yes  type: number  sql: ${TABLE}.user_id ;; }
  dimension: total_amount { type: number  sql: ${TABLE}.total_amount ;; }
}
```
```aml
Model order_totals {
  type: 'query'
  label: 'Order Totals'
  data_source_name: 'demodb'
  models: []

  query: @sql
    SELECT user_id, SUM(amount) AS total_amount
    FROM public.orders
    GROUP BY 1
  ;;

  persistence: FullPersistence {
    schema: 'persisted'
    custom_ddl: @sql
      CREATE TABLE {{ persisted_table }} AS {{ parsed_query }};
      CREATE INDEX {{ index_name }} ON {{ persisted_table }} (user_id);
    ;;
  }

  dimension user_id {
    label: 'User Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.user_id }};;
  }
  dimension total_amount {
    label: 'Total Amount'
    type: 'number'
    definition: @sql {{ #SOURCE.total_amount }};;
  }
}
```

Mark for review; the DDL is not validated. `custom_ddl` works on BigQuery, ClickHouse, MySQL, PostgreSQL,
Redshift, Snowflake and SQL Server only.

## `materialized_view`

Looker: [`materialized_view`](https://docs.cloud.google.com/looker/docs/reference/param-view-materialized-view)

Unsupported. The database refreshes a materialized view, not the tool. Either create the materialized view in the
database and use a table model on it, or emit `FullPersistence` with a schedule. Report the choice.

## `publish_as_db_view`

Looker: [`publish_as_db_view`](https://docs.cloud.google.com/looker/docs/reference/param-view-publish-as-db-view) ·
Holistics: [AML Persistence](https://docs.holistics.io/reference/aml/persistence)

`publish_as_db_view: yes` → `view_name: '<view name>'` in the persistence block. Holistics then keeps a database
view with that name on the latest persisted table.

```aml
persistence: FullPersistence {
  schema: 'persisted'
  view_name: 'user_order_facts'
}
```

Difference: the name of the view changes. Looker names it
`<scratch schema>.<connection key>_<model name>_<view name>`; Holistics names it `<schema>.<view_name>`. Report
that other tools that read the Looker view must be repointed.

## Table optimizations

`indexes`, `sortkeys`, `distribution`, `distribution_style`, `partition_keys`, `cluster_keys`

Looker: [`indexes`](https://docs.cloud.google.com/looker/docs/reference/param-view-indexes),
[`sortkeys`](https://docs.cloud.google.com/looker/docs/reference/param-view-sortkeys),
[`distribution`](https://docs.cloud.google.com/looker/docs/reference/param-view-distribution),
[`distribution_style`](https://docs.cloud.google.com/looker/docs/reference/param-view-distribution-style),
[`partition_keys`](https://docs.cloud.google.com/looker/docs/reference/param-view-partition-keys),
[`cluster_keys`](https://docs.cloud.google.com/looker/docs/reference/param-view-cluster-keys) ·
Holistics: [Persistence Table Optimizations](https://docs.holistics.io/docs/persistence#persistence-table-optimizations)

These apply to persisted tables only. Write them as a `custom_ddl` in the persistence block, in the dialect of the
connection. Without a persistence strategy, drop them.

| LookML | Redshift | BigQuery | Snowflake | PostgreSQL, MySQL, SQL Server |
|---|---|---|---|---|
| `distribution: "c"` | `DISTKEY (c)` | — | — | — |
| `distribution_style: all` / `even` | `DISTSTYLE ALL` / `DISTSTYLE EVEN` | — | — | — |
| `sortkeys: ["a", "b"]` | `SORTKEY (a, b)` | — | — | — |
| `indexes: ["a", "b"]` | `INTERLEAVED SORTKEY (a, b)` | — | — | one `CREATE INDEX` statement per column, after the `CREATE TABLE` |
| `partition_keys: ["d"]` | — | `PARTITION BY d` for a date column; `PARTITION BY DATE(d)` for a timestamp | — | — |
| `cluster_keys: ["a", "b"]` | — | `CLUSTER BY a, b` | `CLUSTER BY (a, b)` | — |

"—" means that the parameter has no effect on that dialect. Drop it.

```lookml
derived_table: {
  sql: … ;;
  datagroup_trigger: ecommerce_etl
  distribution: "user_id"
  sortkeys: ["user_id", "first_order_at"]
}
```

Redshift:

```aml
persistence: FullPersistence {
  schema: 'persisted'
  custom_ddl: @sql
    CREATE TABLE {{ persisted_table }}
    DISTKEY (user_id)
    SORTKEY (user_id, first_order_at)
    AS {{ parsed_query }};
  ;;
}
```

BigQuery, with `partition_keys: ["session_date"]` and `cluster_keys: ["session_date"]`:

```aml
persistence: FullPersistence {
  schema: 'persisted'
  custom_ddl: @sql
    CREATE TABLE {{ persisted_table }}
    PARTITION BY session_date
    CLUSTER BY session_date
    AS {{ parsed_query }};
  ;;
}
```

PostgreSQL, with `indexes: ["created_month"]`:

```aml
persistence: FullPersistence {
  schema: 'persisted'
  custom_ddl: @sql
    CREATE TABLE {{ persisted_table }} AS {{ parsed_query }};
    CREATE INDEX {{ index_name }} ON {{ persisted_table }} (created_month);
  ;;
}
```

Mark for review:

- The DDL text is not validated. Run it once on the target database.
- On an incremental persistence, `custom_ddl` runs for the first build only.
- These parameters change performance only. Dropping them never changes a result.

## `table_compression`, `table_format`

Looker: [`table_compression`](https://docs.cloud.google.com/looker/docs/reference/param-view-table-compression),
[`table_format`](https://docs.cloud.google.com/looker/docs/reference/param-view-table-format)

Unsupported. They apply to Presto and Athena, where Holistics has no `custom_ddl`. Drop them and report them.
