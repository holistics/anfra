# Data source connections

Each entry in `.anfra/data_sources.yml` is a `type` and a `connection`. Anfra passes `connection`
to the database driver unchanged, so its keys must match exactly:

- **A misspelled key is ignored**, and surfaces only as a missing required key or a refused login.
- **`port` is a number** (`port: 5432`). A quoted `"5432"` fails with `invalid config for dbtype`.
  Host-based types have no default port: always set it.
- **Certificates and keys are the PEM text, not a file path.** Paste them in a YAML `|` block.
- **Names to get right:** `user` (not `username`); `dbname` (not `database`), except Athena
  (`database`) and Oracle (`service_name`); Snowflake's `host` is the full account host (there is
  no `account` key).
- **SSL/TLS options cannot be set from this file yet.** Keys such as `ssl_root_cert`, `ssl_cert`
  and `ssl_key` are accepted but have no effect; each driver uses its default.

## By type

| `type` | Required | Optional |
|---|---|---|
| `postgresql` | `host`, `port`, `user`, `dbname` | `password` |
| `redshift` | `host`, `port`, `user`, `dbname` | `password` |
| `mysql` | `host`, `port`, `user`, `dbname` | `password` |
| `sqlserver` | `host` (a named instance can go here), `port`, `user`, `dbname` | `password` |
| `oracledb` | `host`, `port`, `user`, `service_name` | `password` |
| `clickhouse` | `host`, `port`, `user`, `dbname` | `password` |
| `prestodb` | `host`, `port`, `user`, `catalog` | `password` |
| `snowflake` | `host`, `port` (443), `user`, `dbname`, `warehouse`, and `password` or `private_key` | `application` |
| `bigquery` | `project_id`, `credentials_json` | |
| `aws_athena` | `region`, `access_key_id`, `secret_access_key`, `result_bucket_url` | `database`, `catalog` (default `AwsDataCatalog`), `work_group` |
| `databricks` | `host`, `port` (443), `http_path`, `catalog` | `access_token` |
| `motherduck` | `token` | `dbname`, `default_dbname` |
| `duckdb` | | `path`  |

The type names are exact: `aws_athena`, `prestodb`, `oracledb` (not `athena`, `presto`, `oracle`).
A Trino cluster is reached as `prestodb`, but through the Presto driver.

## Examples

```yaml
data_sources:
  warehouse:
    type: postgresql
    connection:
      host: db.internal
      port: 5432
      user: analyst
      password: <password>
      dbname: analytics
```

**Snowflake.** Key-pair auth takes an unencrypted PKCS#8 key (`-----BEGIN PRIVATE KEY-----`); there
is no passphrase key, and no `role` or `schema` key. With no `private_key`, `password` is used.

```yaml
  snowflake:
    type: snowflake
    connection:
      host: xy12345.ap-southeast-1.snowflakecomputing.com
      port: 443
      user: ANALYST
      dbname: ANALYTICS
      warehouse: COMPUTE_WH
      private_key: |
        -----BEGIN PRIVATE KEY-----
        ...
        -----END PRIVATE KEY-----
```

**BigQuery.** `credentials_json` is a service account key's JSON, as text. A key file path or
application default credentials do not work.

```yaml
  bq:
    type: bigquery
    connection:
      project_id: my-project
      credentials_json: |
        {"type": "service_account", "project_id": "my-project", "private_key": "...", "client_email": "..."}
```

**Athena.** Static access keys only: no profile, role or session token.

```yaml
  athena:
    type: aws_athena
    connection:
      region: ap-southeast-1
      access_key_id: <key id>
      secret_access_key: <secret>
      result_bucket_url: s3://my-bucket/athena-results/
      database: analytics
```

**DuckDB.** Give an absolute `path`: a relative one resolves against the query engine's working
directory, not the repo. An in-memory database (no `path`) fails under `anfra serve`, because it pools
connections.

```yaml
  local:
    type: duckdb
    connection:
      path: /home/me/data/shop.duckdb
```
