# External SQL

mxcli can connect to external databases (PostgreSQL, Oracle, SQL Server) for querying, schema exploration, data import, and Database Connector generation. This enables workflows like importing reference data from external systems, exploring database schemas, and auto-generating Mendix integration code.

## Supported Databases

| Database | Driver Names | DSN Format |
|----------|-------------|------------|
| PostgreSQL | `postgres`, `pg`, `postgresql` | `postgres://user:pass@host:5432/dbname` |
| Oracle | `oracle`, `ora` | `oracle://user:pass@host:1521/service` |
| SQL Server | `sqlserver`, `mssql` | `sqlserver://user:pass@host:1433?database=dbname` |

## Security

Credentials are isolated from session output. The DSN (which contains username and password) is never displayed in session output, logs, or error messages. Only the alias and driver name are shown.

## Capabilities

| Feature | Command | Description |
|---------|---------|-------------|
| Connect | `SQL CONNECT` | Establish a named connection |
| Explore | `SQL <alias> SHOW TABLES/VIEWS` | Browse database schema |
| Describe | `SQL <alias> DESCRIBE <table>` | View column details |
| Query | `SQL <alias> <any-sql>` | Run arbitrary SQL |
| Import | `IMPORT FROM <alias>` | Import data into Mendix app DB |
| Generate | `SQL <alias> GENERATE CONNECTOR` | Auto-generate Database Connector MDL |
| Disconnect | `SQL DISCONNECT <alias>` | Close connection |

## Quick Start

```sql
-- Connect to an external database
SQL CONNECT postgres 'postgres://user:pass@localhost:5432/mydb' AS source;

-- Explore the schema
SQL source SHOW TABLES;
SQL source DESCRIBE employees;

-- Query data
SQL source SELECT * FROM employees WHERE active = true LIMIT 10;

-- Import into Mendix
IMPORT FROM source QUERY 'SELECT name, email FROM employees'
  INTO HR.Employee
  MAP (name AS Name, email AS Email);

-- Generate Database Connector MDL
SQL source GENERATE CONNECTOR INTO HRModule TABLES (employees, departments) EXEC;

-- Disconnect
SQL DISCONNECT source;
```

## CLI Subcommand

For one-off queries without the REPL:

```bash
mxcli sql --driver postgres --dsn 'postgres://user:pass@localhost:5432/mydb' "SELECT * FROM users"
```

The subcommand is **read-only by default**. A statement that is not a read
(`INSERT`, `UPDATE`, `DELETE`, `MERGE`, DDL, procedure calls) is refused, and a
read runs in a read-only transaction that is rolled back, so a write inside a
read — a data-modifying `WITH` — fails instead of changing data. Pass `--write`
to change the database; the output then reports the rows affected:

```bash
mxcli sql --alias mydb --write "DELETE FROM orders WHERE id = 42"
```

## Related Pages

- [SQL CONNECT](sql-connect.md) -- Connection syntax and driver details
- [Querying External Databases](sql-query.md) -- Query syntax and schema exploration
- [IMPORT FROM](import-from.md) -- Data import pipeline
- [Credential Management](credentials.md) -- Environment variables and YAML config
- [Database Connector Generation](connector-generation.md) -- Auto-generating integration code
