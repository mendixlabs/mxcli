// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// --- OQL Query Plan data structures ---

type oqlPlanData struct {
	Format     string          `json:"format"`
	Type       string          `json:"type"`
	EntityName string          `json:"entityName"`
	OqlQuery   string          `json:"oqlQuery"`
	Tables     []oqlPlanTable  `json:"tables"`
	Joins      []oqlPlanJoin   `json:"joins"`
	Columns    []oqlPlanColumn `json:"columns"`
	GroupBy    string          `json:"groupBy,omitempty"`
}

type oqlPlanTable struct {
	ID         string        `json:"id"`
	Entity     string        `json:"entity"`
	Alias      string        `json:"alias"`
	JoinType   string        `json:"joinType"`
	Attributes []oqlPlanAttr `json:"attributes"`
	Filters    []string      `json:"filters"`
	Width      float64       `json:"width"`
	Height     float64       `json:"height"`
}

type oqlPlanAttr struct {
	Name        string `json:"name"`
	Expression  string `json:"expression"`
	Alias       string `json:"alias"`
	IsAggregate bool   `json:"isAggregate,omitempty"`
}

type oqlPlanJoin struct {
	ID        string  `json:"id"`
	LeftID    string  `json:"leftId"`
	RightID   string  `json:"rightId"`
	JoinType  string  `json:"joinType"`
	Condition string  `json:"condition"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
}

type oqlPlanColumn struct {
	Expression string `json:"expression"`
	Alias      string `json:"alias"`
}

// OqlQueryPlanELK generates a query plan visualization for a view entity's OQL query.
func OqlQueryPlanELK(ctx *ExecContext, qualifiedName string, entity *domainmodel.Entity) error {
	plan := parseOqlPlan(qualifiedName, entity.OqlQuery)

	out, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return mdlerrors.NewBackend("marshal json", err)
	}
	fmt.Fprint(ctx.Output, string(out))
	return nil
}

// parseOqlPlan parses an OQL query string into a query plan structure.
// It uses the ANTLR parser to get a structured parse of the OQL query,
// then builds the plan from the parsed data.
func parseOqlPlan(entityName, oql string) oqlPlanData {
	plan := oqlPlanData{
		Format:     "elk",
		Type:       "oql-queryplan",
		EntityName: entityName,
		OqlQuery:   oql,
	}

	parsed := ParseOQLString(oql)
	if parsed == nil {
		return plan
	}

	plan = buildPlanFromParsed(entityName, oql, parsed)

	return plan
}

// buildPlanFromParsed constructs an oqlPlanData from a structured OQLParsed.
func buildPlanFromParsed(entityName, oql string, parsed *ast.OQLParsed) oqlPlanData {
	plan := oqlPlanData{
		Format:     "elk",
		Type:       "oql-queryplan",
		EntityName: entityName,
		OqlQuery:   oql,
	}

	// Build columns from parsed SELECT items
	for _, sel := range parsed.Select {
		plan.Columns = append(plan.Columns, oqlPlanColumn{
			Expression: sel.Expression,
			Alias:      sel.Alias,
		})
	}

	// Build tables from parsed FROM + JOINs
	for i, tableRef := range parsed.Tables {
		t := oqlPlanTable{
			ID:       fmt.Sprintf("table-%d", i),
			Entity:   tableRef.Entity,
			Alias:    tableRef.Alias,
			JoinType: tableRef.JoinType,
		}
		// For subquery-derived tables, populate from the inner query
		if tableRef.Subquery != nil {
			if t.Entity != "" {
				t.Entity = "(From) " + t.Entity
			} else {
				t.Entity = "(Subquery)"
			}
			// Attribute inner SELECT items as columns of this table
			for _, sel := range tableRef.Subquery.Select {
				name := sel.Alias
				if name == "" {
					name = sel.Expression
				}
				t.Attributes = append(t.Attributes, oqlPlanAttr{
					Name:        name,
					Expression:  sel.Expression,
					Alias:       sel.Alias,
					IsAggregate: sel.IsAggregate,
				})
			}
			// Attribute inner WHERE as filters
			if tableRef.Subquery.Where != "" {
				conditions := splitOnTopLevelAnd(tableRef.Subquery.Where)
				for _, cond := range conditions {
					cond = strings.TrimSpace(cond)
					if cond != "" {
						t.Filters = append(t.Filters, cond)
					}
				}
			}
		}
		plan.Tables = append(plan.Tables, t)
	}

	// Attribute each SELECT column to its table
	attributeColumnsToTables(plan.Columns, plan.Tables)

	// Add scalar subquery tables from SELECT items (after FROM/JOIN tables
	// so they get unique IDs and don't interfere with column attribution)
	for i, sel := range parsed.Select {
		if sel.Subquery != nil {
			subTable := buildScalarSubqueryTable(sel, i, len(plan.Tables))
			plan.Tables = append(plan.Tables, subTable)
		}
	}

	// Distribute WHERE filters to tables
	if parsed.Where != "" {
		distributeFilters(parsed.Where, plan.Tables)
	}

	// GROUP BY
	if parsed.GroupBy != "" {
		plan.GroupBy = parsed.GroupBy
	}

	// Build join nodes from parsed tables (ON conditions already in OQLTableRef)
	// Pass the number of FROM/JOIN tables so scalar subquery tables are excluded
	plan.Joins = buildJoinsFromParsed(plan.Tables[:len(parsed.Tables)], parsed.Tables)

	// Calculate node dimensions
	for i := range plan.Tables {
		calculateTableDimensions(&plan.Tables[i])
	}
	for i := range plan.Joins {
		calculateJoinDimensions(&plan.Joins[i])
	}

	return plan
}

// buildJoinsFromParsed creates join nodes as a left-deep chain:
//
//	table-0 → [join-0] → [join-1] → [join-2] → result
//	             ↑           ↑           ↑
//	          table-1     table-2     table-3
//
// The first join takes table-0 (left) and table-1 (right).
// Each subsequent join takes the previous join's output (left) and the next table (right).
func buildJoinsFromParsed(planTables []oqlPlanTable, parsedTables []ast.OQLTableRef) []oqlPlanJoin {
	var joins []oqlPlanJoin
	if len(planTables) < 2 {
		return joins
	}

	for i := 1; i < len(planTables); i++ {
		joinType := planTables[i].JoinType
		if joinType == "from" {
			joinType = "join"
		}

		condition := ""
		if i < len(parsedTables) {
			condition = parsedTables[i].OnExpr
		}

		joinID := fmt.Sprintf("join-%d", i-1)

		// Left input: first join uses table-0, subsequent joins chain from previous join
		leftID := planTables[0].ID
		if i > 1 {
			leftID = fmt.Sprintf("join-%d", i-2)
		}

		joins = append(joins, oqlPlanJoin{
			ID:        joinID,
			LeftID:    leftID,
			RightID:   planTables[i].ID,
			JoinType:  joinType,
			Condition: condition,
		})
	}

	return joins
}

// attributeColumnsToTables assigns SELECT columns to their source tables
// based on alias prefix matching (e.g., "o.Discount" → table with alias "o").
func attributeColumnsToTables(columns []oqlPlanColumn, tables []oqlPlanTable) {
	for _, col := range columns {
		expr := col.Expression

		// Check for aggregate function wrapping
		isAggregate := false
		upperExpr := strings.ToUpper(strings.TrimSpace(expr))
		for _, fn := range []string{"count", "sum", "avg", "min", "max"} {
			if strings.HasPrefix(upperExpr, fn+"(") {
				isAggregate = true
				break
			}
		}

		// Extract the alias reference from the expression
		// Patterns: alias.attr, fn(alias.attr), fn(alias/Assoc.Name)
		aliasPattern := regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)`)
		matches := aliasPattern.FindAllStringSubmatch(expr, -1)

		matched := false
		for _, m := range matches {
			alias := m[1]
			attrName := m[2]
			for i := range tables {
				if tables[i].Alias == alias {
					tables[i].Attributes = append(tables[i].Attributes, oqlPlanAttr{
						Name:        attrName,
						Expression:  expr,
						Alias:       col.Alias,
						IsAggregate: isAggregate,
					})
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}

		// If no alias match (e.g., COUNT(*)), attribute to first table
		if !matched && len(tables) > 0 {
			tables[0].Attributes = append(tables[0].Attributes, oqlPlanAttr{
				Name:        col.Alias,
				Expression:  expr,
				Alias:       col.Alias,
				IsAggregate: isAggregate,
			})
		}
	}
}

// distributeFilters splits a WHERE clause into conditions and assigns each
// to the table whose alias it references.
func distributeFilters(whereClause string, tables []oqlPlanTable) {
	// Split on top-level AND
	conditions := splitOnTopLevelAnd(whereClause)

	for _, cond := range conditions {
		cond = strings.TrimSpace(cond)
		if cond == "" {
			continue
		}

		attributed := false
		for i := range tables {
			if tables[i].Alias != "" && strings.Contains(cond, tables[i].Alias+".") {
				tables[i].Filters = append(tables[i].Filters, cond)
				attributed = true
				break
			}
		}
		// If we can't attribute, put on first table
		if !attributed && len(tables) > 0 {
			tables[0].Filters = append(tables[0].Filters, cond)
		}
	}
}

// splitOnTopLevelAnd splits a string on AND keywords at parenthesis depth 0.
func splitOnTopLevelAnd(s string) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	upper := strings.ToUpper(s)

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '(':
			depth++
			current.WriteByte(ch)
		case ')':
			depth--
			current.WriteByte(ch)
		default:
			if depth == 0 && i+3 <= len(upper) {
				if upper[i:i+3] == "and" {
					prevOk := i == 0 || !isIdentChar(s[i-1])
					nextOk := i+3 >= len(s) || !isIdentChar(s[i+3])
					if prevOk && nextOk {
						parts = append(parts, current.String())
						current.Reset()
						i += 2 // skip "ND"
						continue
					}
				}
			}
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// calculateTableDimensions computes width and height for a table node.
func calculateTableDimensions(t *oqlPlanTable) {
	// Header: entity name
	maxTextLen := float64(len(t.Entity))
	if aliasLen := float64(len(t.Alias) + 3); aliasLen > maxTextLen {
		// "Entity (alias)"
		maxTextLen = float64(len(t.Entity)) + aliasLen
	}

	// Attribute rows
	for _, attr := range t.Attributes {
		displayName := attr.Alias
		if attr.IsAggregate {
			displayName = attr.Expression
		}
		if attr.Name != "" && displayName == "" {
			displayName = attr.Name
		}
		lineLen := float64(len(displayName))
		if lineLen > maxTextLen {
			maxTextLen = lineLen
		}
	}

	// Filter rows
	for _, f := range t.Filters {
		// Truncate long filters for sizing
		display := f
		if len(display) > 40 {
			display = display[:37] + "..."
		}
		lineLen := float64(len(display) + 2) // filter icon
		if lineLen > maxTextLen {
			maxTextLen = lineLen
		}
	}

	t.Width = maxTextLen*elkCharWidth + elkHPadding
	if t.Width < elkMinWidth {
		t.Width = elkMinWidth
	}

	rows := len(t.Attributes) + len(t.Filters)
	if rows == 0 {
		rows = 1
	}
	t.Height = elkHeaderHeight + float64(rows)*elkAttrLineHeight
}

// calculateJoinDimensions computes width and height for a join node.
func calculateJoinDimensions(j *oqlPlanJoin) {
	label := strings.ToUpper(j.JoinType)
	textLen := float64(len(label))

	// Condition text below
	if j.Condition != "" {
		condLen := float64(len(j.Condition))
		if condLen > textLen {
			textLen = condLen
		}
	}

	j.Width = textLen*elkCharWidth + elkHPadding
	if j.Width < 80 {
		j.Width = 80
	}
	j.Height = 44
	if j.Condition != "" {
		j.Height = 60
	}
}

// buildScalarSubqueryTable creates a plan table for a scalar subquery in a SELECT item.
// These appear as inline queries like: SELECT (SELECT COUNT(*) FROM Mod.Entity WHERE ...) AS Cnt
func buildScalarSubqueryTable(sel ast.OQLSelectItem, selectIndex, tableOffset int) oqlPlanTable {
	sub := sel.Subquery
	entity := "(Scalar)"
	if sub != nil && len(sub.Tables) > 0 {
		entity = "(Select) " + sub.Tables[0].Entity
	}

	alias := sel.Alias
	if alias == "" {
		alias = fmt.Sprintf("subq%d", selectIndex)
	}

	t := oqlPlanTable{
		ID:       fmt.Sprintf("table-%d", tableOffset),
		Entity:   entity,
		Alias:    alias,
		JoinType: "scalar",
	}

	if sub != nil {
		// Inner SELECT items become attributes
		for _, innerSel := range sub.Select {
			name := innerSel.Alias
			if name == "" {
				name = innerSel.Expression
			}
			t.Attributes = append(t.Attributes, oqlPlanAttr{
				Name:        name,
				Expression:  innerSel.Expression,
				Alias:       innerSel.Alias,
				IsAggregate: innerSel.IsAggregate,
			})
		}

		// Inner WHERE becomes filters
		if sub.Where != "" {
			conditions := splitOnTopLevelAnd(sub.Where)
			for _, cond := range conditions {
				cond = strings.TrimSpace(cond)
				if cond != "" {
					t.Filters = append(t.Filters, cond)
				}
			}
		}
	}

	return t
}

// --- Executor method wrapper for backward compatibility ---
