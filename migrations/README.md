# Database Migrations

This directory contains database schema migrations for the project.

## Structure

- **`postgres/`** - PostgreSQL migrations
- **`clickhouse/`** - ClickHouse migrations

## Migration Naming Convention

```
{version}_{description}.{up|down}.sql
```

Examples:
- `000001_init_schema.up.sql`
- `000001_init_schema.down.sql`
- `000002_add_users_table.up.sql`

## Running Migrations

```bash
# Run PostgreSQL migrations
./scripts/migrate.sh

# Or use golang-migrate tool
migrate -path migrations/postgres -database "postgres://localhost:5432/tiny?sslmode=disable" up
```

## Creating New Migrations

```bash
# Create new migration
migrate create -ext sql -dir migrations/postgres -seq add_new_table
```
