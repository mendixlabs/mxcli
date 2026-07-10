// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
	sqllib "github.com/JordtenBulte-OLC/mxcli/sql"
)

// ensureSQLManager lazily initializes the SQL connection manager.
func ensureSQLManager(ctx *ExecContext) *sqllib.Manager {
	return ctx.ensureSqlMgr()
}

// getOrAutoConnect returns an existing connection or auto-connects using connections.yaml.
func getOrAutoConnect(ctx *ExecContext, alias string) (*sqllib.Connection, error) {
	mgr := ensureSQLManager(ctx)
	conn, err := mgr.Get(alias)
	if err == nil {
		return conn, nil
	}

	// Not connected yet — try auto-connect from config
	if acErr := autoConnect(ctx, alias); acErr != nil {
		return nil, mdlerrors.NewNotFoundMsg("connection", alias, fmt.Sprintf("no connection '%s' (and auto-connect failed: %v)", alias, acErr))
	}
	return mgr.Get(alias)
}

// execSQLConnect handles SQL CONNECT <driver> '<dsn>' AS <alias>
// and SQL CONNECT <alias> (resolve from connections.yaml).
func execSQLConnect(ctx *ExecContext, s *ast.SQLConnectStmt) error {
	if s.DSN == "" && s.Driver == "" {
		// Short form: SQL CONNECT <alias> — resolve from config
		return autoConnect(ctx, s.Alias)
	}

	driver, err := sqllib.ParseDriver(s.Driver)
	if err != nil {
		return err
	}

	mgr := ensureSQLManager(ctx)
	if err := mgr.Connect(driver, s.DSN, s.Alias); err != nil {
		return err
	}

	fmt.Fprintf(ctx.Output, "Connected to %s database as '%s'\n", driver, s.Alias)
	return nil
}

// autoConnect resolves a connection alias from env vars or .mxcli/connections.yaml
// and connects automatically.
func autoConnect(ctx *ExecContext, alias string) error {
	rc, err := sqllib.ResolveConnection(sqllib.ResolveOptions{Alias: alias})
	if err != nil {
		return fmt.Errorf("cannot resolve connection '%s': %w\nAdd it to .mxcli/connections.yaml or use: sql connect <driver> '<dsn>' as %s", alias, err, alias)
	}

	mgr := ensureSQLManager(ctx)
	if err := mgr.Connect(rc.Driver, rc.DSN, alias); err != nil {
		return err
	}

	fmt.Fprintf(ctx.Output, "Connected to %s database as '%s' (from config)\n", rc.Driver, alias)
	return nil
}

// execSQLDisconnect handles SQL DISCONNECT <alias>
func execSQLDisconnect(ctx *ExecContext, s *ast.SQLDisconnectStmt) error {
	mgr := ensureSQLManager(ctx)
	if err := mgr.Disconnect(s.Alias); err != nil {
		return err
	}

	fmt.Fprintf(ctx.Output, "Disconnected '%s'\n", s.Alias)
	return nil
}

// execSQLConnections handles SQL CONNECTIONS
func execSQLConnections(ctx *ExecContext) error {
	mgr := ensureSQLManager(ctx)
	infos := mgr.List()

	if len(infos) == 0 {
		fmt.Fprintln(ctx.Output, "No active sql connections")
		return nil
	}

	// Sort by alias for stable output
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Alias < infos[j].Alias
	})

	result := &sqllib.QueryResult{
		Columns: []string{"Alias", "Driver"},
	}
	for _, info := range infos {
		result.Rows = append(result.Rows, []any{info.Alias, string(info.Driver)})
	}
	sqllib.FormatTable(ctx.Output, result)
	return nil
}

// execSQLQuery handles SQL <alias> <raw-sql>
func execSQLQuery(ctx *ExecContext, s *ast.SQLQueryStmt) error {
	conn, err := getOrAutoConnect(ctx, s.Alias)
	if err != nil {
		return err
	}

	goCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := sqllib.Execute(goCtx, conn, s.Query)
	if err != nil {
		return err
	}

	sqllib.FormatTable(ctx.Output, result)
	fmt.Fprintf(ctx.Output, "(%d rows)\n", len(result.Rows))
	return nil
}

// execSQLShowTables handles SQL <alias> SHOW TABLES
func execSQLShowTables(ctx *ExecContext, s *ast.SQLShowTablesStmt) error {
	conn, err := getOrAutoConnect(ctx, s.Alias)
	if err != nil {
		return err
	}

	goCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := sqllib.ShowTables(goCtx, conn)
	if err != nil {
		return err
	}

	sqllib.FormatTable(ctx.Output, result)
	fmt.Fprintf(ctx.Output, "(%d tables)\n", len(result.Rows))
	return nil
}

// execSQLShowViews handles SQL <alias> SHOW VIEWS
func execSQLShowViews(ctx *ExecContext, s *ast.SQLShowViewsStmt) error {
	conn, err := getOrAutoConnect(ctx, s.Alias)
	if err != nil {
		return err
	}

	goCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := sqllib.ShowViews(goCtx, conn)
	if err != nil {
		return err
	}

	sqllib.FormatTable(ctx.Output, result)
	fmt.Fprintf(ctx.Output, "(%d views)\n", len(result.Rows))
	return nil
}

// execSQLShowFunctions handles SQL <alias> SHOW FUNCTIONS
func execSQLShowFunctions(ctx *ExecContext, s *ast.SQLShowFunctionsStmt) error {
	conn, err := getOrAutoConnect(ctx, s.Alias)
	if err != nil {
		return err
	}

	goCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := sqllib.ShowFunctions(goCtx, conn)
	if err != nil {
		return err
	}

	sqllib.FormatTable(ctx.Output, result)
	fmt.Fprintf(ctx.Output, "(%d functions)\n", len(result.Rows))
	return nil
}

// execSQLGenerateConnector handles SQL <alias> GENERATE CONNECTOR INTO <module> [TABLES (...)] [VIEWS (...)] [EXEC]
func execSQLGenerateConnector(ctx *ExecContext, s *ast.SQLGenerateConnectorStmt) error {
	conn, err := getOrAutoConnect(ctx, s.Alias)
	if err != nil {
		return err
	}

	goCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cfg := &sqllib.GenerateConfig{
		Conn:   conn,
		Module: s.Module,
		Alias:  s.Alias,
		Tables: s.Tables,
		Views:  s.Views,
	}

	result, err := sqllib.GenerateConnector(goCtx, cfg)
	if err != nil {
		return err
	}

	// Report skipped columns
	for _, skip := range result.SkippedCols {
		fmt.Fprintf(ctx.Output, "-- warning: skipped unmappable column: %s\n", skip)
	}

	if s.Exec {
		// Execute constants + entities (parseable by mxcli)
		fmt.Fprintf(ctx.Output, "Generating connector (%d tables, %d views)...\n",
			result.TableCount, result.ViewCount)
		if err := executeGeneratedMDL(ctx, result.ExecutableMDL); err != nil {
			return err
		}
		// Print DATABASE CONNECTION as reference (not yet executable)
		fmt.Fprintf(ctx.Output, "\n-- Database Connection definition (configure in Studio Pro with Database Connector module):\n")
		fmt.Fprint(ctx.Output, result.ConnectionMDL)
		return nil
	}

	// Print complete MDL to output
	fmt.Fprint(ctx.Output, result.MDL)
	fmt.Fprintf(ctx.Output, "\n-- Generated: %d tables, %d views\n", result.TableCount, result.ViewCount)
	return nil
}

// executeGeneratedMDL parses and executes MDL text as if it were a script.
func executeGeneratedMDL(ctx *ExecContext, mdl string) error {
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		return mdlerrors.NewBackend("parse generated MDL", fmt.Errorf("%v", errs[0]))
	}
	if ctx.ExecuteProgramFn == nil {
		return mdlerrors.NewBackend("execute generated MDL", fmt.Errorf("ExecuteProgramFn not set — ExecContext was not created via Executor dispatch"))
	}
	return ctx.ExecuteProgramFn(prog)
}

// execSQLDescribeTable handles SQL <alias> DESCRIBE <table>
func execSQLDescribeTable(ctx *ExecContext, s *ast.SQLDescribeTableStmt) error {
	conn, err := getOrAutoConnect(ctx, s.Alias)
	if err != nil {
		return err
	}

	goCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := sqllib.DescribeTable(goCtx, conn, s.Table)
	if err != nil {
		return err
	}

	sqllib.FormatTable(ctx.Output, result)
	fmt.Fprintf(ctx.Output, "(%d columns)\n", len(result.Rows))
	return nil
}

// Executor wrappers for unmigrated callers.
