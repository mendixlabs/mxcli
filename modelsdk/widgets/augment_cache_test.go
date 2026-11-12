// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// pedAppProject is the Studio Pro-authored fixture, whose widgets/ holds the
// packages its templates are augmented from.
func pedAppProject() string {
	return filepath.Join("..", "..", "testdata", "pedapp", "PedApp.mpr")
}

// augmentableWidgets is every embedded template the fixture has a package for.
func augmentableWidgets(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, id := range ListAvailableTemplates() {
		path, err := mpk.FindMPK(filepath.Dir(pedAppProject()), id)
		if err == nil && path != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) < 5 {
		t.Fatalf("the fixture has packages for %d embedded templates, expected the data grid and its filters at least: %v", len(ids), ids)
	}
	return ids
}

// withoutIDs is tmpl's type and object with every 32-character ID replaced by
// its order of first appearance, walking keys in sorted order. Augmenting
// mints an ID for each property it adds, so two augmentations of the same
// template differ in those and in nothing else; a load replaces every ID
// again (collectIDs), so what must be the same is everything but them.
func withoutIDs(tmpl *WidgetTemplate) []any {
	seen := map[string]int{}
	var walk func(v any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			out := make([]any, 0, 2*len(keys))
			for _, k := range keys {
				out = append(out, k, walk(x[k]))
			}
			return out
		case []any:
			out := make([]any, len(x))
			for i, e := range x {
				out[i] = walk(e)
			}
			return out
		case string:
			if len(x) != 32 {
				return x
			}
			if _, ok := seen[x]; !ok {
				seen[x] = len(seen)
			}
			return fmt.Sprintf("id#%d", seen[x])
		}
		return v
	}
	return []any{tmpl.WidgetID, tmpl.Name, tmpl.Version, tmpl.Generated, tmpl.StableIds, walk(tmpl.Type), walk(tmpl.Object)}
}

func augmentWithoutCache(tmpl *WidgetTemplate, id string) *WidgetTemplate {
	saved := augmentCacheOn
	augmentCacheOn = false
	defer func() { augmentCacheOn = saved }()
	return augmentFromMPK(tmpl, id, pedAppProject())
}

// Every pluggable widget instance loaded its template with
// GetTemplateFullBSON, and every load deep-cloned the embedded template and
// augmented the clone from the project's widget package: the same clone and
// the same augmentation for every data grid and every filter of a script
// (ako/mxcli#2160). The augmented template depends on the embedded template
// and the parsed package alone, so it is made once for the pair and read by
// every load.
//
// That is only sound while a load reads the template and changes nothing in
// it. So the kept template is compared with a fresh augmentation before any
// load, and again after several.
func TestAugmentFromMPK_IsMadeOncePerTemplateAndPackage(t *testing.T) {
	n := 0
	gen := func() string { n++; return fmt.Sprintf("%032x", n) }
	for _, id := range augmentableWidgets(t) {
		tmpl, err := GetTemplate(id)
		if err != nil || tmpl == nil {
			t.Fatalf("%s: no embedded template: %v", id, err)
		}
		fresh := augmentWithoutCache(tmpl, id)
		if fresh == tmpl {
			t.Fatalf("control: %s was not augmented, so there is nothing to keep", id)
		}
		if !reflect.DeepEqual(withoutIDs(fresh), withoutIDs(augmentWithoutCache(tmpl, id))) {
			t.Fatalf("control: two fresh augmentations of %s differ in more than their IDs, so the comparison below proves nothing", id)
		}
		kept := augmentFromMPK(tmpl, id, pedAppProject())
		if !reflect.DeepEqual(withoutIDs(kept), withoutIDs(fresh)) {
			t.Fatalf("%s: the kept template is not what a fresh augmentation gives", id)
		}

		before := augmentations.Load()
		for i := 0; i < 4; i++ {
			typ, obj, _, _, _, err := GetTemplateFullBSON(id, gen, pedAppProject())
			if err != nil || typ == nil || obj == nil {
				t.Fatalf("%s: load %d: %v", id, i, err)
			}
		}
		if made := augmentations.Load() - before; made != 0 {
			t.Errorf("%s: four loads augmented the template %d more times, want 0", id, made)
		}
		again := augmentFromMPK(tmpl, id, pedAppProject())
		if again != kept {
			t.Errorf("%s: a later load was given another template", id)
		}
		if !reflect.DeepEqual(withoutIDs(again), withoutIDs(fresh)) {
			t.Fatalf("%s: loading changed the kept template", id)
		}
	}
}

// Control for the count: without the cache every load augments.
func TestAugmentFromMPK_WithoutTheCacheEveryLoadAugments(t *testing.T) {
	id := augmentableWidgets(t)[0]
	tmpl, _ := GetTemplate(id)
	before := augmentations.Load()
	for i := 0; i < 4; i++ {
		augmentWithoutCache(tmpl, id)
	}
	if made := augmentations.Load() - before; made != 4 {
		t.Fatalf("four uncached augmentations counted %d, so the count is not measuring them", made)
	}
}
