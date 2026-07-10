// SPDX-License-Identifier: Apache-2.0

// Bug 8: a pluggable-widget datasource `sort by <attr> desc` describes as `asc`.
//
// Root cause: a Pages/Forms$GridSortItem stores its direction under the BSON key
// SortDirection (what Studio Pro and the modelsdk engine — the default — write),
// but the DESCRIBE readers looked up the SortOrder key (only correct for the
// unrelated Microflows$SortItem / DocumentTemplates$GridSortItem metamodel types).
// The lookup missed, so every column fell back to the default "asc".
package executor

import "testing"

// gridSortWidget builds a minimal DataGrid2/Gallery widget map whose
// CustomWidgetXPathSource carries one sort column with the given direction stored
// under the given key — mirroring the on-disk BSON for a pluggable widget.
func gridSortWidget(sortKey, sortValue string) map[string]any {
	return map[string]any{
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{
					"Value": map[string]any{
						"DataSource": map[string]any{
							"$Type":     "CustomWidgets$CustomWidgetXPathSource",
							"EntityRef": map[string]any{"Entity": "MyModule.Foo"},
							"SortBar": map[string]any{
								"SortItems": []any{
									int32(2), // BSON non-empty-array version marker
									map[string]any{
										"AttributeRef": map[string]any{"Attribute": "MyModule.Foo.Bar"},
										sortKey:        sortValue,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestDataGrid2DataSource_SortDirectionDescending(t *testing.T) {
	// Studio Pro / modelsdk engine storage name.
	ds := extractDataGrid2DataSource(nil, gridSortWidget("SortDirection", "Descending"))
	if ds == nil {
		t.Fatal("expected a datasource, got nil")
	}
	if len(ds.SortColumns) != 1 {
		t.Fatalf("expected 1 sort column, got %d", len(ds.SortColumns))
	}
	if got := ds.SortColumns[0].Order; got != "desc" {
		t.Errorf("sort direction dropped: got %q, want %q", got, "desc")
	}
}

func TestDataGrid2DataSource_SortOrderLegacyFallback(t *testing.T) {
	// Legacy sdk/mpr writer stored the (misnamed) SortOrder key; reading it back
	// must still work so pre-fix files round-trip.
	ds := extractDataGrid2DataSource(nil, gridSortWidget("SortOrder", "Descending"))
	if ds == nil || len(ds.SortColumns) != 1 {
		t.Fatalf("expected 1 sort column, got %#v", ds)
	}
	if got := ds.SortColumns[0].Order; got != "desc" {
		t.Errorf("legacy SortOrder fallback dropped: got %q, want %q", got, "desc")
	}
}
