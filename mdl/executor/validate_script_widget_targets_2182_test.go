// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// scriptTargetViolations runs the MDL-ALTER02 pass over src, against ctx (nil:
// no project).
func scriptTargetViolations(t *testing.T, ctx *ExecContext, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return validateScriptWidgetTargets(prog, func() *ExecContext { return ctx })
}

func joinedMessages(vs []linter.Violation) string {
	var b strings.Builder
	for _, v := range vs {
		b.WriteString(v.RuleID + ": " + v.Message + "\n")
	}
	return b.String()
}

const cardPage = `create page MyModule.Check_Expand (Title: 'Check', Layout: Atlas_Core.Atlas_Default) {
	use building block MyModule.Card as a_
};
`

// TestCheckExpandsBlockForLaterAlterTarget is ako/mxcli#2182: a later ALTER
// naming a widget of a placed block was never resolved, so a name the block
// does not hold passed check and exec then stopped on `widget … not found`.
// MyModule.Card holds `container` and `cardtitle`; placed `as a_` they are
// a_container and a_cardtitle.
func TestCheckExpandsBlockForLaterAlterTarget(t *testing.T) {
	ctx := cardBlockCtx(t)

	bad := scriptTargetViolations(t, ctx, cardPage+
		`alter page MyModule.Check_Expand { set (Class: 'mb') on a_container99 };`)
	msg := joinedMessages(bad)
	if len(bad) != 1 || !strings.Contains(msg, `widget "a_container99" not found`) {
		t.Fatalf("misspelt block widget not reported; got:\n%s", msg)
	}
	if !strings.Contains(msg, "did you mean `a_container`") {
		t.Errorf("no suggestion of the expanded name; got:\n%s", msg)
	}

	// Control: the names the block really holds, by its prefix.
	for _, name := range []string{"a_container", "a_cardtitle"} {
		if vs := scriptTargetViolations(t, ctx, cardPage+
			`alter page MyModule.Check_Expand { set (Class: 'mb') on `+name+` };`); len(vs) > 0 {
			t.Errorf("%s exists after expansion and was refused:\n%s", name, joinedMessages(vs))
		}
	}
	// The unprefixed name is the block's, not the page's.
	if vs := scriptTargetViolations(t, ctx, cardPage+
		`alter page MyModule.Check_Expand { drop widget container };`); len(vs) != 1 {
		t.Errorf("the unprefixed name was accepted; got:\n%s", joinedMessages(vs))
	}
}

// A block inserted by an ALTER is expanded the same way, and its names are
// targets for the operations after it — in the same statement and later ones.
func TestCheckExpandsBlockInsertedByAlter(t *testing.T) {
	ctx := cardBlockCtx(t)
	src := `create page MyModule.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
		container c1
	};
	alter page MyModule.P {
		insert after c1 { use building block MyModule.Card as b_ }
		set (Class: 'x') on b_container
	};
	alter page MyModule.P { set (Class: 'y') on b_cardtitle };
	`
	if vs := scriptTargetViolations(t, ctx, src); len(vs) > 0 {
		t.Errorf("names of an inserted block refused:\n%s", joinedMessages(vs))
	}
	vs := scriptTargetViolations(t, ctx, src+`alter page MyModule.P { set (Class: 'z') on b_cardtitel };`)
	if len(vs) != 1 || !strings.Contains(vs[0].Message, `widget "b_cardtitel" not found`) {
		t.Errorf("misspelt inserted block widget not reported; got:\n%s", joinedMessages(vs))
	}
}

// `create or modify page` writes the statement's tree, block expanded.
func TestCheckExpandsBlockInCreateOrModify(t *testing.T) {
	ctx := cardBlockCtx(t)
	src := `create or modify page MyModule.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
		use building block MyModule.Card as a_
		use building block MyModule.Card as b_
	};
	alter page MyModule.P { set (Class: 'x') on b_container; drop widget c_container };
	`
	vs := scriptTargetViolations(t, ctx, src)
	if len(vs) != 1 || !strings.Contains(vs[0].Message, `widget "c_container" not found`) {
		t.Errorf("want exactly c_container refused; got:\n%s", joinedMessages(vs))
	}
}

// A plain widget of a script-created page was not resolved either.
func TestCheckScriptPageMisspeltPlainWidget(t *testing.T) {
	src := `create page MyModule.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { container c1 };
	alter page MyModule.P { set (Class: 'x') on c99 };`
	if vs := scriptTargetViolations(t, nil, src); len(vs) != 1 {
		t.Errorf("want c99 refused with no project; got:\n%s", joinedMessages(vs))
	}
	ok := `create page MyModule.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { container c1 };
	alter page MyModule.P { set Name = 'c2' on c1 };
	alter page MyModule.P { set (Class: 'x') on c2 };`
	if vs := scriptTargetViolations(t, nil, ok); len(vs) > 0 {
		t.Errorf("a renamed widget was refused:\n%s", joinedMessages(vs))
	}
}

// Without a project the block cannot be expanded: names under its prefix are
// unknown, not absent, and must not be refused; an unprefixed block leaves
// every name possible.
func TestCheckUnexpandedBlockAllowsItsNames(t *testing.T) {
	if vs := scriptTargetViolations(t, nil, cardPage+
		`alter page MyModule.Check_Expand { set (Class: 'mb') on a_container99 };`); len(vs) > 0 {
		t.Errorf("a name under an unexpanded prefix was refused:\n%s", joinedMessages(vs))
	}
	// Outside the prefix the page's own names still decide.
	if vs := scriptTargetViolations(t, nil, cardPage+
		`alter page MyModule.Check_Expand { set (Class: 'mb') on zz };`); len(vs) != 1 {
		t.Errorf("a name outside every prefix was not refused; got:\n%s", joinedMessages(vs))
	}
	unprefixed := `create page MyModule.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
		use building block MyModule.Card
	};
	alter page MyModule.P { set (Class: 'mb') on anything };`
	if vs := scriptTargetViolations(t, nil, unprefixed); len(vs) > 0 {
		t.Errorf("an unexpanded unprefixed block refused a name:\n%s", joinedMessages(vs))
	}
}

// A DataGrid 2 column is addressed by a name derived from its attribute, not
// the one the script writes, so a page holding one cannot be judged by name.
func TestCheckScriptPageWithGridColumnsIsNotJudged(t *testing.T) {
	src := `create page MyModule.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
		datagrid dg (DataSource: database from MyModule.E) { column colName (Attribute: Name) }
	};
	alter page MyModule.P { set (Class: 'x') on Name };`
	if vs := scriptTargetViolations(t, nil, src); len(vs) > 0 {
		t.Errorf("a derived column name was refused:\n%s", joinedMessages(vs))
	}
}

// `if not exists` may keep the stored page, whose names the script cannot see.
func TestCheckScriptPageIfNotExistsIsNotJudged(t *testing.T) {
	src := `create page if not exists MyModule.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { container c1 };
	alter page MyModule.P { set (Class: 'x') on c99 };`
	if vs := scriptTargetViolations(t, nil, src); len(vs) > 0 {
		t.Errorf("a page that may be the stored one was judged:\n%s", joinedMessages(vs))
	}
}

// ako/mxcli#559, the other side of the same gap, through the full check entry:
// the same block twice with different prefixes is not a duplicate.
func TestCheckBlockTwiceWithPrefixesThroughCheck(t *testing.T) {
	src := `create page MyModule.Dash (Title: 'Dash', Layout: Atlas_Core.Atlas_Default) {
		use building block MyModule.Card as a_
		use building block MyModule.Card as b_
	};
	alter page MyModule.Dash { set (Class: 'x') on a_container; set (Class: 'y') on b_container };`
	ctx := cardBlockCtx(t)
	if vs := scriptTargetViolations(t, ctx, src); len(vs) > 0 {
		t.Errorf("targets of two prefixed copies refused:\n%s", joinedMessages(vs))
	}
	for _, e := range pageContextErrorsOf(t, ctx, src) {
		if strings.Contains(e, "CE0495") {
			t.Errorf("prefixed copies reported as duplicates: %s", e)
		}
	}
}

// storedPageWithCardCtx is alterRefsCtx's stored page (MyModule.P_Scopes, with
// tbTop) in a project that also holds the MyModule.Card building block.
func storedPageWithCardCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "P_Scopes")
	bb := mkBuildingBlock(mod.ID, "Card")
	h := mkHierarchy(mod)
	withContainer(h, bb.ContainerID, mod.ID)
	deps := &countingDeps{}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListBuildingBlocksFunc: func() ([]*pages.BuildingBlock, error) {
			return []*pages.BuildingBlock{bb}, nil
		},
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) { return bbRawWidgets(), nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(storedScopesPage(), unitID, deps), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

// On a STORED page the names a block inserts were counted as one widget named
// after the block, so a later operation on one of its real names was refused
// (false), while a SET on a name the block does not hold was waived (missed):
// both read the script's additions before expansion.
func TestCheckStoredPageBlockInsertNames(t *testing.T) {
	ctx := storedPageWithCardCtx(t)
	insert := `alter page MyModule.P_Scopes { insert after tbTop { use building block MyModule.Card as s_ } };
	`
	// Control: exec runs both — the block's widgets are s_container / s_cardtitle.
	if errs := checkAlterPageRefs(t, ctx, insert+
		`alter page MyModule.P_Scopes { insert after s_container { container zz } };`); len(errs) > 0 {
		t.Errorf("a target the inserted block holds was refused: %v", errs)
	}
	if errs := checkAlterSet(t, ctx, insert+
		`alter page MyModule.P_Scopes { set (Class: 'x') on s_cardtitle };`); len(errs) > 0 {
		t.Errorf("a SET on a widget the inserted block holds was refused: %v", errs)
	}
	errs := checkAlterSet(t, ctx, insert+`alter page MyModule.P_Scopes { set (Class: 'x') on s_cardtitel };`)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), `"s_cardtitel" not found`) {
		t.Errorf("a SET on a name the inserted block does not hold was waived; got %v", errs)
	}
}
