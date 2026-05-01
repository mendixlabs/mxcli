# SQL Integration Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/386)

## Setup

> See [AGENT-TESTING.md](./AGENT-TESTING.md) for build, execution methods, and verification patterns.

**Prerequisites:** Docker Desktop must be running. All tests in this document require a running Mendix app (Method 4: Docker).

---

## 1. SQL CONNECT

### 1.1 Full form — postgres

```
SQL CONNECT postgres 'postgres://postgres:test@localhost:5432/testdb' AS mydb
```

**Expected:** `Connected to postgres database as 'mydb'`

### 1.2 Full form — sqlserver

```
SQL CONNECT sqlserver 'sqlserver://sa:Test1234!@localhost:1433?database=testdb' AS mssqldb
```

**Expected:** `Connected to sqlserver database as 'mssqldb'`

### 1.3 Full form — oracle

```
SQL CONNECT oracle 'oracle://system:test@localhost:1521/FREEPDB1' AS oradb
```

**Expected:** `Connected to oracle database as 'oradb'`

### 1.4 Driver aliases — postgresql

```
SQL CONNECT postgresql 'postgres://postgres:test@localhost:5432/testdb' AS pgdb
```

**Expected:** `Connected to postgres database as 'pgdb'`

### 1.5 Driver aliases — pg

```
SQL CONNECT pg 'postgres://postgres:test@localhost:5432/testdb' AS pgdb2
```

**Expected:** `Connected to postgres database as 'pgdb2'`

### 1.6 Driver aliases — mssql

```
SQL CONNECT mssql 'sqlserver://sa:Test1234!@localhost:1433?database=testdb' AS msdb2
```

**Expected:** `Connected to sqlserver database as 'msdb2'`

### 1.7 Driver aliases — ora

```
SQL CONNECT ora 'oracle://system:test@localhost:1521/FREEPDB1' AS oradb2
```

**Expected:** `Connected to oracle database as 'oradb2'`

### 1.8 Unsupported driver

```
SQL CONNECT mysql 'mysql://root:pass@localhost:3306/test' AS mysqldb
```

**Expected:** Error — unsupported driver `mysql`.

### 1.9 Invalid DSN

```
SQL CONNECT postgres 'not-a-valid-dsn' AS baddb
```

**Expected:** Error — connection failed. Clear message with driver and DSN hint.

### 1.10 Connection timeout

```
SQL CONNECT postgres 'postgres://postgres:test@10.255.255.1:5432/testdb' AS timeoutdb
```

**Expected:** Error — connection timed out. Does not hang indefinitely.

### 1.11 Duplicate alias

```
SQL CONNECT postgres 'postgres://postgres:test@localhost:5432/testdb' AS mydb
SQL CONNECT postgres 'postgres://postgres:test@localhost:5432/testdb' AS mydb
```

**Expected:** Second connect fails or replaces. Verify behavior is consistent.

---

## 2. SQL DISCONNECT

### 2.1 Disconnect active connection

```
SQL CONNECT postgres 'postgres://postgres:test@localhost:5432/testdb' AS mydb
SQL DISCONNECT mydb
```

**Expected:** `Disconnected 'mydb'`

### 2.2 Disconnect non-existent alias

```
SQL DISCONNECT nonexistent
```

**Expected:** Error — no active connection with alias `nonexistent`.

### 2.3 Disconnect then query

```
SQL CONNECT postgres 'postgres://postgres:test@localhost:5432/testdb' AS mydb
SQL DISCONNECT mydb
SQL mydb SELECT 1
```

**Expected:** Error or auto-reconnect (if alias in config). Document actual behavior.

### 2.4 Double disconnect

```
SQL DISCONNECT mydb
SQL DISCONNECT mydb
```

**Expected:** First succeeds, second gives error.

---

## 3. SQL CONNECTIONS

### 3.1 List active connections

```
SQL CONNECT postgres 'postgres://postgres:test@localhost:5432/testdb' AS mydb
SQL CONNECT sqlserver 'sqlserver://sa:Test1234!@localhost:1433?database=testdb' AS mssqldb
SQL CONNECTIONS
```

**Expected:** Table with columns `Alias`, `Driver`. Two rows: `mydb | postgres`, `mssqldb | sqlserver`.

### 3.2 No active connections

```
SQL CONNECTIONS
```

**Expected:** `No active sql connections`

### 3.3 After disconnect

```
SQL CONNECT postgres 'postgres://postgres:test@localhost:5432/testdb' AS mydb
SQL DISCONNECT mydb
SQL CONNECTIONS
```

**Expected:** `No active sql connections`

---

## 4. SQL QUERY

### 4.1 Simple SELECT

```
SQL mydb SELECT * FROM users LIMIT 10
```

**Expected:** Formatted table with columns from `users`. Footer: `(10 rows)`

### 4.2 SELECT with WHERE

```
SQL mydb SELECT name, email FROM users WHERE active = true LIMIT 5
```

**Expected:** Two-column table. `(5 rows)`

### 4.3 Aggregate query

```
SQL mydb SELECT COUNT(*) AS total FROM users
```

**Expected:** Single row, single column. `(1 rows)`

### 4.4 Empty result

```
SQL mydb SELECT * FROM users WHERE name = 'nonexistent_user_xyz'
```

**Expected:** Column headers shown. `(0 rows)`

### 4.5 Auto-connect on query

Configure `mydb` in connections.yaml but do not explicitly connect. Run:

```
SQL mydb SELECT 1 AS test
```

**Expected:** Auto-connects using config, executes query. Output includes result.

### 4.6 Auto-connect failure

Remove `mydb` from connections.yaml and env vars. Run:

```
SQL mydb SELECT 1
```

**Expected:** Error — cannot auto-connect, no config found for alias.

### 4.7 30-second timeout

```
SQL mydb SELECT pg_sleep(60)
```

**Expected:** Error — query timed out after 30 seconds.

### 4.8 Syntax error in SQL

```
SQL mydb SELEC * FORM users
```

**Expected:** Error — SQL syntax error from database. Message includes SQL error detail.

### 4.9 Non-existent table

```
SQL mydb SELECT * FROM nonexistent_table
```

**Expected:** Error — relation does not exist.

### 4.10 Multi-line query result formatting

```
SQL mydb SELECT id, name, email, active, created_at FROM users LIMIT 20
```

**Expected:** Columns aligned. No truncation of values. Readable table format.

---

## 5. SQL SHOW TABLES / VIEWS / FUNCTIONS

### 5.1 SHOW TABLES

```
SQL mydb SHOW TABLES
```

**Expected:** Table listing all tables. Footer: `(N tables)` where N matches actual count.

### 5.2 SHOW VIEWS

```
SQL mydb SHOW VIEWS
```

**Expected:** Table listing all views. Footer: `(N views)`. Includes `active_users` from seed data.

### 5.3 SHOW FUNCTIONS

```
SQL mydb SHOW FUNCTIONS
```

**Expected:** Table listing all functions. Footer: `(N functions)`.

### 5.4 SHOW TABLES — empty database

Connect to a database with no user tables.

**Expected:** Empty result or system tables only. `(0 tables)` or similar.

### 5.5 SHOW TABLES — MSSQL

```
SQL mssqldb SHOW TABLES
```

**Expected:** Same format as postgres. Driver-specific catalog queries handled internally.

### 5.6 SHOW TABLES — Oracle

```
SQL oradb SHOW TABLES
```

**Expected:** Same format as postgres. Oracle catalog queries handled internally.

---

## 6. SQL DESCRIBE

### 6.1 Describe table

```
SQL mydb DESCRIBE users
```

**Expected:** Column metadata table (column name, type, nullable, default, etc.). Footer: `(N columns)`.

### 6.2 Describe view

```
SQL mydb DESCRIBE active_users
```

**Expected:** Column metadata for view columns. Same format as table describe.

### 6.3 Non-existent table

```
SQL mydb DESCRIBE nonexistent_table
```

**Expected:** Error — table/view not found.

### 6.4 Table with many columns

Describe a table with 20+ columns. Verify all columns listed without truncation.

### 6.5 Describe across drivers

Run `SQL <alias> DESCRIBE users` for postgres, mssql, and oracle. Verify consistent output format.

---

## 7. SQL GENERATE CONNECTOR

### 7.1 Dry run — all tables

```
SQL mydb GENERATE CONNECTOR INTO MyModule
```

**Expected:** Prints MDL output. Footer: `-- Generated: N tables, 0 views`

### 7.2 Dry run — specific tables

```
SQL mydb GENERATE CONNECTOR INTO MyModule TABLES (users, orders)
```

**Expected:** MDL for `users` and `orders` only. Footer: `-- Generated: 2 tables, 0 views`

### 7.3 Dry run — tables and views

```
SQL mydb GENERATE CONNECTOR INTO MyModule TABLES (users) VIEWS (active_users)
```

**Expected:** MDL for `users` table and `active_users` view. Footer: `-- Generated: 1 tables, 1 views`

### 7.4 Dry run — views only

```
SQL mydb GENERATE CONNECTOR INTO MyModule VIEWS (active_users)
```

**Expected:** MDL for view. Footer: `-- Generated: 0 tables, 1 views`

### 7.5 EXEC — create entities

```
SQL mydb GENERATE CONNECTOR INTO MyModule TABLES (users, orders) EXEC
```

**Expected:** Executes MDL in-process. Creates entities and constants in the Mendix project. Verify with `show entities in MyModule;`.

### 7.6 EXEC — verify constants created

After §7.5, verify database connection constants created in the module.

### 7.7 120-second timeout

Generate connector for a database with 100+ tables using EXEC.

**Expected:** Completes within 120 seconds or times out with clear error.

### 7.8 Skipped columns — unmappable types

Create a table with a column type that has no Mendix equivalent (e.g., `JSONB`, `ARRAY`, `GEOMETRY`).

```
SQL mydb GENERATE CONNECTOR INTO MyModule TABLES (table_with_exotic_types)
```

**Expected:** Warning listing skipped columns with unmappable types. Generated MDL omits those columns.

### 7.9 Non-existent table in TABLES clause

```
SQL mydb GENERATE CONNECTOR INTO MyModule TABLES (nonexistent)
```

**Expected:** Error or warning — table not found.

### 7.10 Empty TABLES clause

```
SQL mydb GENERATE CONNECTOR INTO MyModule TABLES ()
```

**Expected:** Error — empty table list.

### 7.11 EXEC without Mendix project

Run GENERATE CONNECTOR EXEC without opening a Mendix project.

**Expected:** Error — not connected to a Mendix project.

---

## 8. IMPORT FROM

### 8.1 Simple import

```
IMPORT FROM mydb QUERY 'SELECT id, name, email FROM users LIMIT 100' INTO MyModule.User MAP (id AS Id, name AS Name, email AS Email)
```

**Expected:** Batch progress output. Summary: `Imported 100 rows into MyModule.User`.

### 8.2 Import with BATCH size

```
IMPORT FROM mydb QUERY 'SELECT id, name FROM users' INTO MyModule.User MAP (id AS Id, name AS Name) BATCH 500
```

**Expected:** Processes in batches of 500. Progress output shows batch numbers.

### 8.3 Import with LIMIT

```
IMPORT FROM mydb QUERY 'SELECT id, name FROM users' INTO MyModule.User MAP (id AS Id, name AS Name) LIMIT 50
```

**Expected:** Imports exactly 50 rows regardless of query result size.

### 8.4 Import with LINK — raw object ID

```
IMPORT FROM mydb QUERY 'SELECT o.id, o.total, o.user_id FROM orders o LIMIT 100' INTO MyModule.Order MAP (id AS Id, total AS Total) LINK (user_id TO Order_User)
```

**Expected:** Link set using raw `user_id` value as object ID. Summary includes link statistics.

### 8.5 Import with LINK ON — lookup

```
IMPORT FROM mydb QUERY 'SELECT o.id, o.total, o.user_id FROM orders o LIMIT 100' INTO MyModule.Order MAP (id AS Id, total AS Total) LINK (user_id TO Order_User ON UserId)
```

**Expected:** Lookup cache built for `UserId` attribute. Links resolved via lookup. Summary includes cache hit/miss statistics.

### 8.6 Auto-connect to Mendix app DB

Run IMPORT FROM. Verify `_mendix` connection auto-created for the app's database.

### 8.7 Requires Mendix project

Run IMPORT FROM without opening a Mendix project.

**Expected:** Error — not connected to a Mendix project.

### 8.8 ReferenceSet not supported

```
IMPORT FROM mydb QUERY 'SELECT ...' INTO MyModule.Entity MAP (...) LINK (col TO MyReferenceSet)
```

Where `MyReferenceSet` is a ReferenceSet association.

**Expected:** Error — ReferenceSet not supported in IMPORT LINK.

### 8.9 Default batch size

Run IMPORT without BATCH clause.

**Expected:** Default batch size is 1000.

### 8.10 10-minute timeout

```
IMPORT FROM mydb QUERY 'SELECT * FROM large_table' INTO MyModule.Entity MAP (...) LIMIT 500000
```

**Expected:** Completes within 10 minutes or times out with clear error and partial import summary.

### 8.11 Empty query result

```
IMPORT FROM mydb QUERY 'SELECT id, name FROM users WHERE 1=0' INTO MyModule.User MAP (id AS Id, name AS Name)
```

**Expected:** `Imported 0 rows into MyModule.User`. No error.

### 8.12 Column mismatch in MAP

```
IMPORT FROM mydb QUERY 'SELECT id, name FROM users LIMIT 10' INTO MyModule.User MAP (id AS Id, nonexistent_col AS Name)
```

**Expected:** Error — column `nonexistent_col` not in query result.

### 8.13 BATCH and LIMIT together

```
IMPORT FROM mydb QUERY 'SELECT id, name FROM users' INTO MyModule.User MAP (id AS Id, name AS Name) BATCH 100 LIMIT 250
```

**Expected:** 3 batches (100 + 100 + 50). Total: 250 rows.

---

## 9. CREATE DATABASE CONNECTION

### 9.1 Minimal — PostgreSQL

```
CREATE DATABASE CONNECTION MyModule.MyPostgresDB
  TYPE 'PostgreSQL'
  CONNECTION STRING @MyModule.PostgresDSN
BEGIN
END;
```

**Expected:** Database connection created. Verify with `SHOW DATABASE CONNECTIONS`.

### 9.2 With literal connection string

```
CREATE DATABASE CONNECTION MyModule.MyDB
  TYPE 'PostgreSQL'
  CONNECTION STRING 'postgres://user:pass@host:5432/db'
BEGIN
END;
```

**Expected:** Database connection created with literal DSN.

### 9.3 With USERNAME and PASSWORD

```
CREATE DATABASE CONNECTION MyModule.MyDB
  TYPE 'PostgreSQL'
  CONNECTION STRING @MyModule.PostgresDSN
  USERNAME @MyModule.DBUser
  PASSWORD @MyModule.DBPass
BEGIN
END;
```

**Expected:** Database connection created with separate credentials.

### 9.4 With QUERY definitions

```
CREATE DATABASE CONNECTION MyModule.MyDB
  TYPE 'PostgreSQL'
  CONNECTION STRING @MyModule.PostgresDSN
BEGIN
  QUERY GetUsers
    SQL 'SELECT * FROM users WHERE active = {isActive}'
    PARAMETER isActive : Boolean DEFAULT true
    RETURNS MyModule.User
    MAP (
      name AS Name,
      email AS Email
    );
  QUERY GetOrdersByStatus
    SQL 'SELECT * FROM orders WHERE status = {status} AND total > {minTotal}'
    PARAMETER status : String
    PARAMETER minTotal : Decimal DEFAULT 0
    RETURNS MyModule.Order
    MAP (
      total AS Total,
      status AS Status
    );
END;
```

**Expected:** Database connection created with two queries. Parameters resolved. DESCRIBE shows full definition.

### 9.5 QUERY parameter types

Test each parameter type:

| Type | Example |
|------|---------|
| String | `PARAMETER name : String` |
| Integer | `PARAMETER count : Integer` |
| Long | `PARAMETER bigId : Long` |
| Decimal | `PARAMETER amount : Decimal` |
| Boolean | `PARAMETER active : Boolean` |
| DateTime | `PARAMETER since : DateTime` |
| Date | `PARAMETER day : Date` |

**Expected:** All types accepted and preserved in DESCRIBE.

### 9.6 QUERY with NULL modifier

```
QUERY FindUser
  SQL 'SELECT * FROM users WHERE name = {name}'
  PARAMETER name : String NULL
  RETURNS MyModule.User
  MAP (name AS Name);
```

**Expected:** NULL modifier preserved. Parameter allows null values.

### 9.7 QUERY with DEFAULT

```
QUERY ListUsers
  SQL 'SELECT * FROM users WHERE active = {isActive}'
  PARAMETER isActive : Boolean DEFAULT true
  RETURNS MyModule.User
  MAP (name AS Name);
```

**Expected:** Default value preserved in DESCRIBE.

### 9.8 Database types — MSSQL

```
CREATE DATABASE CONNECTION MyModule.MssqlDB
  TYPE 'MSSQL'
  CONNECTION STRING @MyModule.MssqlDSN
BEGIN
END;
```

**Expected:** Type `MSSQL` accepted and preserved.

### 9.9 Database types — Oracle

```
CREATE DATABASE CONNECTION MyModule.OracleDB
  TYPE 'Oracle'
  CONNECTION STRING @MyModule.OracleDSN
BEGIN
END;
```

**Expected:** Type `Oracle` accepted and preserved.

### 9.10 Write guard

Attempt CREATE DATABASE CONNECTION without opening a Mendix project.

**Expected:** Error — not connected to a Mendix project.

---

## 10. SHOW DATABASE CONNECTIONS

### 10.1 List all connections

```
SHOW DATABASE CONNECTIONS;
```

**Expected:** Table with columns: `Qualified Name`, `Module`, `Name`, `Folder`, `Type`, `Queries`.

### 10.2 No connections

Run on a project with no database connections.

**Expected:** Empty table or message.

### 10.3 Multiple connections

Create 3 database connections across 2 modules. Verify all listed with correct metadata.

---

## 11. DESCRIBE DATABASE CONNECTION

### 11.1 Simple connection

```
DESCRIBE DATABASE CONNECTION MyModule.MyDB;
```

**Expected:** Reproduces full MDL definition including TYPE, CONNECTION STRING, and all QUERY blocks.

### 11.2 Connection with queries

Create connection with 2+ queries (see §9.4). Run DESCRIBE. Verify all queries, parameters, return types, and MAP blocks present.

### 11.3 Non-existent connection

```
DESCRIBE DATABASE CONNECTION MyModule.NonExistent;
```

**Expected:** Clear error — database connection not found.

### 11.4 Roundtrip

1. CREATE DATABASE CONNECTION with queries
2. DESCRIBE → capture MDL
3. DROP DATABASE CONNECTION
4. Execute captured MDL
5. DESCRIBE again → compare

**Expected:** Identical or cosmetic-only differences.

---

## 12. MULTI-STEP WORKFLOWS

### 12.1 Full SQL exploration workflow

1. `SQL CONNECT postgres 'postgres://...' AS mydb`
2. `SQL mydb SHOW TABLES`
3. `SQL mydb DESCRIBE users`
4. `SQL mydb SELECT * FROM users LIMIT 5`
5. `SQL mydb GENERATE CONNECTOR INTO MyModule TABLES (users, orders) EXEC`
6. `show entities in MyModule;` — verify entities created
7. `IMPORT FROM mydb QUERY 'SELECT id, name, email FROM users LIMIT 100' INTO MyModule.User MAP (id AS Id, name AS Name, email AS Email)`

**Expected:** Each step succeeds. Entities visible after GENERATE CONNECTOR EXEC. Import populates data.

### 12.2 Multi-driver workflow

1. Connect postgres, mssql, and oracle simultaneously
2. `SQL CONNECTIONS` — verify all three listed
3. Run SHOW TABLES on each
4. Run DESCRIBE on same table across all three
5. Disconnect all three

**Expected:** All drivers work in parallel. Output formats consistent across drivers.

### 12.3 Generate then import

1. `SQL mydb GENERATE CONNECTOR INTO TestModule TABLES (users, orders) EXEC`
2. Verify entities created
3. `IMPORT FROM mydb QUERY 'SELECT id, name FROM users LIMIT 50' INTO TestModule.User MAP (id AS Id, name AS Name)`
4. `IMPORT FROM mydb QUERY 'SELECT id, user_id, total FROM orders LIMIT 100' INTO TestModule.Order MAP (id AS Id, total AS Total) LINK (user_id TO Order_User ON UserId)`

**Expected:** Full pipeline works end-to-end. Links resolve correctly.

### 12.4 Database connection with live queries

1. CREATE DATABASE CONNECTION with parameterized queries
2. DESCRIBE — verify definition
3. SHOW DATABASE CONNECTIONS — verify listed
4. Use generated entities in a nanoflow/microflow

**Expected:** Full lifecycle from connection definition to usage.

---

## 13. FAILURE MODES & ERROR RECOVERY

### 13.1 Unsupported driver

```
SQL CONNECT mysql 'mysql://root:pass@localhost:3306/db' AS mysqldb
```

**Expected:** Error — unsupported driver. Lists supported drivers.

### 13.2 Connection timeout

```
SQL CONNECT postgres 'postgres://user:pass@10.255.255.1:5432/db' AS deadhost
```

**Expected:** Error — connection timed out. Does not block REPL.

### 13.3 Auto-connect failure — no config

```
SQL unknown_alias SELECT 1
```

**Expected:** Error — no connection config found for `unknown_alias`.

### 13.4 Invalid DSN format

```
SQL CONNECT postgres 'not://valid' AS baddsn
```

**Expected:** Error — invalid DSN.

### 13.5 Feature gate — SQL without feature flag

If SQL features require a feature gate/flag, test without it enabled.

**Expected:** Clear error indicating the feature is not available.

### 13.6 ReferenceSet in IMPORT LINK

Attempt to LINK to a ReferenceSet association.

**Expected:** Error — ReferenceSet not supported.

### 13.7 Empty child table warning

Run IMPORT FROM where the LINK target table has zero matching rows.

**Expected:** Warning about empty lookup results. Import completes with zero links.

### 13.8 Query after disconnect

```
SQL CONNECT postgres '...' AS mydb
SQL DISCONNECT mydb
SQL mydb SELECT 1
```

**Expected:** Error or auto-reconnect. Consistent behavior.

### 13.9 GENERATE CONNECTOR — non-existent module target

```
SQL mydb GENERATE CONNECTOR INTO NonExistent.Sub.Module EXEC
```

**Expected:** Module auto-created or clear error.

### 13.10 IMPORT FROM — entity not found

```
IMPORT FROM mydb QUERY 'SELECT 1 AS id' INTO NonExistent.Entity MAP (id AS Id)
```

**Expected:** Error — entity not found.

---

## 14. BOUNDARY & STRESS

### 14.1 Large table — 100+ columns

Create a test table with 100+ columns. Run:

```
SQL mydb DESCRIBE wide_table
SQL mydb GENERATE CONNECTOR INTO MyModule TABLES (wide_table)
```

**Expected:** All columns listed in DESCRIBE. Generated MDL includes all mappable columns.

### 14.2 IMPORT with 100K+ rows

```
IMPORT FROM mydb QUERY 'SELECT id, name FROM users' INTO MyModule.User MAP (id AS Id, name AS Name)
```

With 100K+ rows in users table.

**Expected:** Completes within 10-minute timeout. Batch progress visible. Memory stable.

### 14.3 Concurrent connections — 5+ databases

Connect to 5 databases simultaneously. Run queries on each. Verify isolation — queries on one connection do not affect others.

### 14.4 GENERATE CONNECTOR with 50+ tables

```
SQL mydb GENERATE CONNECTOR INTO BigModule
```

On a database with 50+ tables.

**Expected:** All tables processed. MDL output complete. EXEC completes within 120 seconds.

### 14.5 Large query result — 10K+ rows

```
SQL mydb SELECT * FROM orders
```

With 10K+ rows.

**Expected:** All rows returned. No truncation. Performance reasonable.

### 14.6 IMPORT with complex MAP and LINK

```
IMPORT FROM mydb QUERY 'SELECT o.id, o.total, o.status, o.user_id, u.name AS user_name FROM orders o JOIN users u ON o.user_id = u.id' INTO MyModule.Order MAP (id AS Id, total AS Total, status AS Status) LINK (user_id TO Order_User ON UserId) BATCH 200 LIMIT 5000
```

**Expected:** Join query works. MAP and LINK coexist. Batch + limit respected.

### 14.7 Rapid connect/disconnect cycle

```
-- repeat 10 times:
SQL CONNECT postgres '...' AS tempdb
SQL DISCONNECT tempdb
```

**Expected:** No resource leak. `SQL CONNECTIONS` shows zero after all disconnects.

### 14.8 Special characters in DSN

```
SQL CONNECT postgres 'postgres://user:p%40ss%23word@localhost:5432/test%20db' AS specialdb
```

**Expected:** URL-encoded characters handled correctly.

---

## Test Project Coverage Matrix

| Category | Enquiries | Evora Factory | Lato Inventory |
|---|---|---|---|
| SQL CONNECT (3 drivers) | All 3 | All 3 | All 3 |
| SQL SHOW TABLES/VIEWS | All 3 drivers | All 3 drivers | All 3 drivers |
| SQL DESCRIBE | 5+ tables | 5+ tables | 5+ tables |
| SQL QUERY (sample 10) | Various queries | Various queries | Various queries |
| GENERATE CONNECTOR dry run | 2+ tables | 2+ tables | 2+ tables |
| GENERATE CONNECTOR EXEC | 2 tables + verify | 2 tables + verify | 2 tables + verify |
| IMPORT FROM | 3 imports with LINK | 3 imports with LINK | 3 imports with LINK |
| CREATE DATABASE CONNECTION | 2 connections | 2 connections | 2 connections |
| DESCRIBE DATABASE CONNECTION | Roundtrip | Roundtrip | Roundtrip |
| Multi-step workflows (§12) | Full pipeline | Full pipeline | Full pipeline |
| Failure modes (§13) | Per driver | Per driver | Per driver |
| Boundary/Stress (§14) | Select cases | Select cases | Select cases |

---

## Automated Test Coverage

| Area | Tests | Status |
|---|---|---|
| SQL CONNECT — all drivers | — | **Manual only** |
| SQL DISCONNECT | — | **Manual only** |
| SQL CONNECTIONS | — | **Manual only** |
| SQL QUERY — basic | — | **Manual only** |
| SQL QUERY — timeout | — | **Manual only** |
| SQL SHOW TABLES/VIEWS/FUNCTIONS | — | **Manual only** |
| SQL DESCRIBE | — | **Manual only** |
| SQL GENERATE CONNECTOR — dry run | — | **Manual only** |
| SQL GENERATE CONNECTOR — EXEC | — | **Manual only** |
| IMPORT FROM — basic | — | **Manual only** |
| IMPORT FROM — LINK/LINK ON | — | **Manual only** |
| IMPORT FROM — BATCH/LIMIT | — | **Manual only** |
| CREATE DATABASE CONNECTION | — | **Manual only** |
| SHOW DATABASE CONNECTIONS | — | **Manual only** |
| DESCRIBE DATABASE CONNECTION | — | **Manual only** |
| Roundtrip (DATABASE CONNECTION) | — | **Manual only** |
| Multi-step workflows (§12) | — | **Manual only** |
| Failure modes (§13) | — | **Manual only** |
| Boundary/Stress (§14) | — | **Manual only** |

Manual testing priority:
1. SQL CONNECT across all 3 drivers — validates driver layer
2. GENERATE CONNECTOR EXEC → verify entities — validates MDL generation pipeline
3. IMPORT FROM with LINK ON — validates lookup cache and data pipeline
4. Multi-step workflow §12.1 — highest integration risk
5. Failure modes §13.1–§13.10 — error handling quality

---

## Manual Test Report Template

Copy and fill in after running manual tests.

```markdown
## Manual Testing — SQL Integration

**Date:** YYYY-MM-DD
**Build:** `make build && make test && make lint-go` — PASS

### Database Setup

| Driver | Version | Container | Seeded |
|--------|---------|-----------|--------|
| PostgreSQL | 16 | docker | ✅/❌ |
| MSSQL | 2022 | docker | ✅/❌ |
| Oracle | 23 Free | docker | ✅/❌ |

### Test Projects

| App | Studio Pro | GENERATE CONNECTOR EXEC | IMPORT FROM |
|-----|-----------|------------------------|-------------|
| Lato Enquiry Management | 11.4.0 | ✅/❌ | ✅/❌ |
| Evora Factory Management | 10.24.15 | ✅/❌ | ✅/❌ |
| Lato Product Inventory | 11.2.0 | ✅/❌ | ✅/❌ |

### Command Coverage

| Command | postgres | sqlserver | oracle | Notes |
|---------|----------|-----------|--------|-------|
| SQL CONNECT (full form) | ✅/❌ | ✅/❌ | ✅/❌ | |
| SQL DISCONNECT | ✅/❌ | ✅/❌ | ✅/❌ | |
| SQL CONNECTIONS | ✅/❌ | — | — | |
| SQL QUERY | ✅/❌ | ✅/❌ | ✅/❌ | |
| SQL SHOW TABLES | ✅/❌ | ✅/❌ | ✅/❌ | |
| SQL SHOW VIEWS | ✅/❌ | ✅/❌ | ✅/❌ | |
| SQL SHOW FUNCTIONS | ✅/❌ | ✅/❌ | ✅/❌ | |
| SQL DESCRIBE | ✅/❌ | ✅/❌ | ✅/❌ | |
| SQL GENERATE CONNECTOR | ✅/❌ | ✅/❌ | ✅/❌ | |
| SQL GENERATE CONNECTOR EXEC | ✅/❌ | ✅/❌ | ✅/❌ | |
| IMPORT FROM | ✅/❌ | ✅/❌ | ✅/❌ | |
| IMPORT FROM with LINK | ✅/❌ | ✅/❌ | ✅/❌ | |
| IMPORT FROM with LINK ON | ✅/❌ | ✅/❌ | ✅/❌ | |
| CREATE DATABASE CONNECTION | ✅/❌ | ✅/❌ | ✅/❌ | |
| SHOW DATABASE CONNECTIONS | ✅/❌ | — | — | |
| DESCRIBE DATABASE CONNECTION | ✅/❌ | — | — | |

### Multi-Step Workflows (§12)

| Scenario | Result | Notes |
|----------|--------|-------|
| 12.1 Full SQL exploration | ✅/❌ | |
| 12.2 Multi-driver | ✅/❌ | |
| 12.3 Generate then import | ✅/❌ | |
| 12.4 Database connection lifecycle | ✅/❌ | |

### Failure Modes (§13)

| Scenario | Result | Notes |
|----------|--------|-------|
| 13.1 Unsupported driver | ✅/❌ | |
| 13.2 Connection timeout | ✅/❌ | |
| 13.3 Auto-connect failure | ✅/❌ | |
| 13.4 Invalid DSN | ✅/❌ | |
| 13.5 Feature gate | ✅/❌ | |
| 13.6 ReferenceSet in LINK | ✅/❌ | |
| 13.7 Empty child table | ✅/❌ | |
| 13.8 Query after disconnect | ✅/❌ | |
| 13.9 Non-existent module target | ✅/❌ | |
| 13.10 Entity not found | ✅/❌ | |

### Boundary & Stress (§14)

| Scenario | Result | Notes |
|----------|--------|-------|
| 14.1 100+ columns | ✅/❌ | |
| 14.2 100K+ row import | ✅/❌ | |
| 14.3 5+ concurrent connections | ✅/❌ | |
| 14.4 50+ table GENERATE | ✅/❌ | |
| 14.5 10K+ row query | ✅/❌ | |
| 14.7 Rapid connect/disconnect x10 | ✅/❌ | |

### Issues Found

1. (none / describe issues here)
```
