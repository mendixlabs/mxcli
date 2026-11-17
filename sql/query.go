// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"fmt"
)

// QueryResult holds the result of a SQL query.
type QueryResult struct {
	Columns []string
	Rows    [][]any
	// RowsAffected is set for a statement run as a write: the rows it changed.
	RowsAffected *int64
}

// Execute runs a SQL query and returns the result.
func Execute(ctx context.Context, conn *Connection, query string) (*QueryResult, error) {
	rows, err := conn.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// scanRows reads every row of rows.
func scanRows(rows *sql.Rows) (*QueryResult, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	result := &QueryResult{
		Columns: cols,
		Rows:    make([][]any, 0),
	}

	for rows.Next() {
		// Create scan targets
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert []byte to string for display
		row := make([]any, len(cols))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = v
			}
		}
		result.Rows = append(result.Rows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return result, nil
}
