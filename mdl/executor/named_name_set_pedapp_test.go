// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// namedNameSet holds only the elements whose own name folds to the target's,
// where loadNameSet holds every element of the kind. A statement looks up one
// name, so the two must answer that lookup alike: compared for every name of
// each kind in the fixture, in its stored spelling, upper-cased, lower-cased
// and under another module, and for names nothing holds.
func TestNamedNameSet_AnswersALookupAsTheFullSetDoes(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = `mdl 1;
create microflow MyFirstModule.NamedProbe_Flow () begin end;
create nanoflow MyFirstModule.NamedProbe_Nano () begin end;
create page MyFirstModule.NamedProbe_Page (title: 'Probe', layout: Atlas_Core.Atlas_Default) {
  container c1 { }
};
`
	if err := runScript(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	ctx := exec.newExecContext(t.Context())

	for _, kind := range []string{"microflow", "nanoflow", "page"} {
		full := loadNameSet(ctx, kind)
		if len(full.exact) < 3 {
			t.Fatalf("%s: the fixture lists %d, too few to compare", kind, len(full.exact))
		}
		var targets []string
		for qn := range full.exact {
			module, short, _ := strings.Cut(qn, ".")
			targets = append(targets, qn, strings.ToUpper(qn), strings.ToLower(qn),
				"NoSuchModule."+short, module+"."+short+"_Absent", module+"."+short[:len(short)-1])
		}
		targets = append(targets, "MyFirstModule.NamedProbe_Flow", "myfirstmodule.namedprobe_nano", "MYFIRSTMODULE.NAMEDPROBE_PAGE")
		found, folded := 0, 0
		for _, target := range targets {
			named := namedNameSet(ctx, kind, target)
			if named == nil {
				t.Fatalf("%s %q: no set", kind, target)
			}
			want, wantOK := full.find(target)
			got, gotOK := named.find(target)
			// Two stored names that differ only in case leave which one the
			// fold finds to map order, in both sets.
			if gotOK != wantOK || !strings.EqualFold(got, want) || (want == target) != (got == target) {
				t.Errorf("%s: find(%q) = %q, %v; the full set gives %q, %v", kind, target, got, gotOK, want, wantOK)
			}
			if wantOK {
				found++
				if want != target {
					folded++
				}
			}
			if len(named.exact) > 4 {
				t.Errorf("%s %q: the set holds %d names; it is not filtered", kind, target, len(named.exact))
			}
		}
		if found == 0 || folded == 0 || found == len(targets) {
			t.Fatalf("%s: %d of %d lookups found a name, %d by folding: the comparison is one-sided", kind, found, len(targets), folded)
		}
	}

	// The platform's own microflows, which no listing holds.
	sys := meta.SystemMicroflows[0]
	for _, target := range []string{sys, strings.ToLower(sys)} {
		want, wantOK := loadNameSet(ctx, "microflow").find(target)
		got, gotOK := namedNameSet(ctx, "microflow", target).find(target)
		if !wantOK || got != want || gotOK != wantOK {
			t.Errorf("find(%q) = %q, %v; the full set gives %q, %v", target, got, gotOK, want, wantOK)
		}
	}

	// A kind it does not cover, and no target: the caller lists.
	if namedNameSet(ctx, "enumeration", "MyFirstModule.X") != nil || namedNameSet(ctx, "microflow", "") != nil || namedNameSet(ctx, "microflow", "Unqualified") != nil {
		t.Error("a set where the caller should list")
	}
}
