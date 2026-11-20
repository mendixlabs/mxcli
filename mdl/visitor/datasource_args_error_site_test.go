// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"
	"testing"
)

// ako/mxcli#533, the part left open after #926: a malformed data-source
// argument reported "line 3:19 no viable alternative at input 'datasource'" —
// the column of a keyword that is correct, while the mistake sat in the
// argument list further along the line. ANTLR predicts a widget property with
// full-LL lookahead, so its error always lands on the property's first token;
// the message must at least name the argument and its column.
func TestDataSourceArgumentErrorNamesTheArgument(t *testing.T) {
	cases := []struct {
		args, at, want string
	}{
		{"Param = ", "Param", "`Param` has no value"},
		{"Param: ", "Param", "`Param` has no value"},
		{"Param $v", "Param", "`Param $v` is missing `=`"},
		{"A = $a, , B = $b", ", B", "an empty argument"},
	}
	for _, c := range cases {
		line := "  dataview DVMain (datasource: microflow M.DS_X(" + c.args + ")) { }"
		src := "create page M.P (title: 'x', layout: Atlas_Core.Atlas_Default) {\n" + line + "\n};"
		_, errs := Build(src)
		var all []string
		for _, e := range errs {
			all = append(all, e.Error())
		}
		got := strings.Join(all, "\n")
		col := strings.Index(line, "M.DS_X(") + len("M.DS_X(") + strings.Index(c.args, strings.TrimPrefix(c.at, ", "))
		if c.at == ", B" {
			col = strings.Index(line, ", ,") + 2 // the empty slot after the first comma
		}
		site := fmt.Sprintf("line 2:%d", col)
		if !strings.Contains(got, site) || !strings.Contains(got, c.want) {
			t.Errorf("(%s): want %q at %q; got:\n%s", c.args, c.want, site, got)
		}
	}
}
