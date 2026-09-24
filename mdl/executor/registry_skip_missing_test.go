// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// DROP ... IF EXISTS skips only the document it names being missing (or its
// module); every other error still fails the statement (mendixlabs/mxcli#1190).
func TestSkipMissingIfAsked(t *testing.T) {
	layout := func(ifExists bool) ast.Statement {
		return &ast.DropLayoutStmt{Name: ast.QualifiedName{Module: "M", Name: "Shell"},
			DropIfExists: ast.DropIfExists{IfExists: ifExists}}
	}
	for _, tc := range []struct {
		name     string
		stmt     ast.Statement
		err      error
		wantErr  bool
		wantNote string
	}{
		{"named layout missing", layout(true), mdlerrors.NewNotFound("layout", "M.Shell"), false, "layout M.Shell does not exist, skipping"},
		{"module missing", layout(true), mdlerrors.NewNotFound("module", "M"), false, "layout M.Shell does not exist, skipping"},
		{"without IF EXISTS", layout(false), mdlerrors.NewNotFound("layout", "M.Shell"), true, ""},
		{"another document missing", layout(true), mdlerrors.NewNotFound("page", "M.Other"), true, ""},
		{"not a not-found", layout(true), errors.New("write failed"), true, ""},
		{"success", layout(true), nil, false, ""},
		{"java action label", &ast.DropJavaActionStmt{Name: ast.QualifiedName{Module: "M", Name: "Act"},
			DropIfExists: ast.DropIfExists{IfExists: true}}, mdlerrors.NewNotFound("java action", "M.Act"), false,
			"java action M.Act does not exist, skipping"},
	} {
		ctx := (&Executor{}).newExecContext(context.Background())
		var out bytes.Buffer
		ctx.Output = &out
		err := skipMissingIfAsked(ctx, tc.stmt, tc.err)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
		if tc.wantNote != "" && !strings.Contains(out.String(), tc.wantNote) {
			t.Errorf("%s: output = %q, want %q", tc.name, out.String(), tc.wantNote)
		}
		if tc.wantNote == "" && out.Len() > 0 {
			t.Errorf("%s: unexpected output %q", tc.name, out.String())
		}
	}
}
