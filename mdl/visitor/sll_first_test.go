// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// repoScripts is every MDL script under mdl-examples.
func repoScripts(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..", "mdl-examples")
	scripts := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".mdl" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scripts[path] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatalf("read the example scripts: %v", err)
	}
	if len(scripts) < 1000 {
		t.Fatalf("found %d scripts under %s, expected the whole collection", len(scripts), root)
	}
	return scripts
}

func buildFullContextOnly(src string) (*ast.Program, []error) {
	saved := sllFirst
	sllFirst = false
	defer func() { sllFirst = saved }()
	return Build(src)
}

func errorTexts(errs []error) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Error()
	}
	return out
}

// Predicting in SLL mode first must change nothing but the time: the same
// program, and for input with a syntax error the same messages, as a parse
// with full LL prediction alone. Compared on every example script as written,
// and cut off in the middle, which is input with a syntax error in most of
// them.
func TestBuild_SLLFirstBuildsWhatFullContextBuilds(t *testing.T) {
	scripts := repoScripts(t)
	clean, failing, sllSufficed := 0, 0, 0
	for path, whole := range scripts {
		for _, src := range []string{whole, whole[:len(whole)*2/3]} {
			wantProg, wantErrs := buildFullContextOnly(src)
			before := fullContextParses.Load()
			gotProg, gotErrs := Build(src)
			fellBack := fullContextParses.Load() != before
			if !reflect.DeepEqual(errorTexts(gotErrs), errorTexts(wantErrs)) {
				t.Fatalf("%s (%d bytes): errors differ\n SLL first: %q\n full LL:   %q", path, len(src), errorTexts(gotErrs), errorTexts(wantErrs))
			}
			if !reflect.DeepEqual(gotProg, wantProg) {
				t.Fatalf("%s (%d bytes): the programs differ", path, len(src))
			}
			if len(wantErrs) == 0 {
				clean++
			} else {
				failing++
			}
			if !fellBack {
				sllSufficed++
			}
		}
	}
	t.Logf("%d inputs: %d without errors, %d with; SLL sufficed for %d", clean+failing, clean, failing, sllSufficed)
	// Controls: both kinds of input, and both paths, must have been compared.
	if clean == 0 || failing == 0 || sllSufficed == 0 || sllSufficed == clean+failing {
		t.Fatalf("the comparison did not cover both paths: %d clean, %d failing, %d parsed by SLL alone", clean, failing, sllSufficed)
	}
}

// A script without syntax errors is not parsed with full LL prediction at
// all; parsed the old way it is, once.
func TestBuild_ACleanScriptNeedsNoFullContextParse(t *testing.T) {
	const src = "mdl 1;\ncreate or modify microflow M.F ($Item: M.E)\nreturns boolean\nbegin\n  if $Item/Name = empty then\n    return false;\n  end if;\n  return true;\nend;\n"
	before := fullContextParses.Load()
	if _, errs := buildFullContextOnly(src); len(errs) > 0 {
		t.Fatalf("the script has errors: %v", errs)
	}
	if got := fullContextParses.Load() - before; got != 1 {
		t.Fatalf("control: the full-LL-only parse counted %d full parses, want 1", got)
	}
	before = fullContextParses.Load()
	if _, errs := Build(src); len(errs) > 0 {
		t.Fatalf("the script has errors: %v", errs)
	}
	if got := fullContextParses.Load() - before; got != 0 {
		t.Errorf("a clean script took %d full-LL parses, want 0", got)
	}
}
