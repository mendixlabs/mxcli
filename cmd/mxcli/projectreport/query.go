// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// Querier is the slice of the catalog a collector reads. *catalog.Catalog
// satisfies it; tests use an in-memory catalog with rows inserted directly.
type Querier interface {
	Query(sql string) (*catalog.QueryResult, error)
}

// rows runs a query and returns each row as strings keyed by column — every
// collector reads names and counts, and converting once keeps the SQL the only
// thing that differs between them.
func rows(q Querier, sql string) ([]map[string]string, error) {
	res, err := q.Query(sql)
	if err != nil {
		return nil, fmt.Errorf("%w\n  query: %s", err, strings.Join(strings.Fields(sql), " "))
	}
	out := make([]map[string]string, 0, len(res.Rows))
	for _, r := range res.Rows {
		m := make(map[string]string, len(res.Columns))
		for i, c := range res.Columns {
			if i < len(r) {
				m[c] = str(r[i])
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprint(t)
	}
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		f, ferr := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if ferr != nil {
			return 0
		}
		return int(f)
	}
	return n
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// sqlList renders names as a quoted SQL IN list.
func sqlList(names []string) string {
	if len(names) == 0 {
		return "''"
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = "'" + strings.ReplaceAll(n, "'", "''") + "'"
	}
	return strings.Join(q, ", ")
}

// moduleOf is the module prefix of a qualified name, "" for an unqualified one.
func moduleOf(qn string) string {
	if i := strings.IndexByte(qn, '.'); i > 0 {
		return qn[:i]
	}
	return ""
}

// ---------------------------------------------------------------- scope

// Scope decides which modules are "the app". Marketplace and platform modules
// are out by default: they dominate god nodes, coupling and coverage with
// infrastructure nobody on the team wrote (ako/mxcli#2222).
type Scope struct {
	App     []string `json:"app"`
	Outside []string `json:"outside"`
}

// ScopeOptions selects the app's modules.
type ScopeOptions struct {
	IncludeMarketplace bool
	Exclude            []string
	Only               []string
}

// alwaysOutside are modules that are never the app's own, even when they carry
// no Marketplace source: the platform module, and the runner's injected one.
var alwaysOutside = map[string]bool{"System": true, "MxTest": true}

// LoadScope reads modules and splits them into the app's and the rest.
func LoadScope(q Querier, opt ScopeOptions) (Scope, error) {
	rs, err := rows(q, "SELECT Name, COALESCE(Source, '') AS Source FROM modules ORDER BY Name")
	if err != nil {
		return Scope{}, err
	}
	only := setOf(opt.Only)
	excl := setOf(opt.Exclude)
	var s Scope
	for _, r := range rs {
		name := r["Name"]
		in := !alwaysOutside[name] && !excl[name] && (opt.IncludeMarketplace || r["Source"] == "")
		if len(only) > 0 {
			in = only[name]
		}
		if in {
			s.App = append(s.App, name)
		} else {
			s.Outside = append(s.Outside, name)
		}
	}
	return s, nil
}

// InApp reports whether a module is one of the app's.
func (s Scope) InApp(module string) bool {
	for _, m := range s.App {
		if m == module {
			return true
		}
	}
	return false
}

func setOf(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
