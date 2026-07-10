// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
)

// listLanguages lists all languages found in the project's translatable strings.
// Requires REFRESH CATALOG FULL to populate the strings table.
func listLanguages(ctx *ExecContext) error {
	if ctx.Catalog == nil {
		return mdlerrors.NewValidation("no catalog available — run refresh catalog full first")
	}

	result, err := ctx.Catalog.Query(`
		select Language, count(*) as StringCount
		from strings
		where Language != ''
		GROUP by Language
		ORDER by StringCount desc
	`)
	if err != nil {
		return mdlerrors.NewBackend("query languages", err)
	}

	if len(result.Rows) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No translatable strings found. Run refresh catalog full to populate the strings table.")
		return nil
	}

	tr := &TableResult{
		Columns: []string{"Language", "Strings"},
		Summary: fmt.Sprintf("(%d languages)", len(result.Rows)),
	}
	for _, row := range result.Rows {
		lang := ""
		count := ""
		if len(row) > 0 {
			lang = fmt.Sprintf("%v", row[0])
		}
		if len(row) > 1 {
			count = fmt.Sprintf("%v", row[1])
		}
		tr.Rows = append(tr.Rows, []any{lang, count})
	}
	return writeResult(ctx, tr)
}

// --- Executor method wrapper for backward compatibility ---
