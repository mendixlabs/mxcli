// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

func TestScanWidgetOwnRefs(t *testing.T) {
	// A DataView referencing an entity via DataSource, with a child action button
	// (a separate widget) whose microflow must NOT be attributed to the DataView.
	dataView := map[string]any{
		"$Type": "Forms$DataView",
		"DataSource": map[string]any{
			"$Type":  "Forms$EntityPathSource",
			"Entity": "Sales.Order",
		},
		"Widgets": []any{ // child widget — excluded from the parent's own refs
			map[string]any{
				"$Type":  "Forms$ActionButton",
				"Action": map[string]any{"Settings": map[string]any{"$Type": "Forms$MicroflowSettings", "Microflow": "Sales.ACT_Child"}},
			},
		},
	}
	ent, mf, nf, _, _ := scanWidgetOwnRefs(dataView)
	if ent != "Sales.Order" {
		t.Errorf("entity = %q, want Sales.Order", ent)
	}
	if mf != "" {
		t.Errorf("microflow = %q, want empty (child button's MF must not leak to parent)", mf)
	}
	if nf != "" {
		t.Errorf("nanoflow = %q, want empty", nf)
	}

	// A button's own microflow + nanoflow.
	button := map[string]any{
		"$Type":  "Forms$ActionButton",
		"Action": map[string]any{"Settings": map[string]any{"Microflow": "M.DoThing"}},
		"Extra":  map[string]any{"Nanoflow": "M.DoNano"},
	}
	if _, mf, nf, _, _ := scanWidgetOwnRefs(button); mf != "M.DoThing" || nf != "M.DoNano" {
		t.Errorf("button refs = (%q,%q), want (M.DoThing, M.DoNano)", mf, nf)
	}

	// Determinism: multiple microflows → the lexicographically smallest, stable
	// across runs regardless of map order.
	multi := map[string]any{
		"A": map[string]any{"Microflow": "M.Zeta"},
		"B": map[string]any{"Microflow": "M.Alpha"},
	}
	for range 5 {
		if _, mf, _, _, _ := scanWidgetOwnRefs(multi); mf != "M.Alpha" {
			t.Fatalf("non-deterministic microflow pick: got %q, want M.Alpha", mf)
		}
	}

	// No refs.
	if e, m, n, p, _ := scanWidgetOwnRefs(map[string]any{"$Type": "Forms$Label"}); e != "" || m != "" || n != "" || p != "" {
		t.Errorf("expected no refs, got (%q,%q,%q)", e, m, n)
	}
}

func TestExtractLayoutRef(t *testing.T) {
	tests := []struct {
		name    string
		rawData map[string]any
		want    string
	}{
		{
			name:    "no FormCall",
			rawData: map[string]any{},
			want:    "",
		},
		{
			name: "string Form field",
			rawData: map[string]any{
				"FormCall": map[string]any{
					"Form": "MyModule.MyLayout",
				},
			},
			want: "MyModule.MyLayout",
		},
		{
			name: "empty Form and no Layout",
			rawData: map[string]any{
				"FormCall": map[string]any{
					"Form": "",
				},
			},
			want: "",
		},
		{
			name: "binary Layout field",
			rawData: map[string]any{
				"FormCall": map[string]any{
					"Layout": bson.Binary{
						Data: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
					},
				},
			},
			want: "04030201-0605-0807-090a-0b0c0d0e0f10",
		},
		{
			// Page templates (Forms$PageTemplate) nest the layout under LayoutCall
			// rather than FormCall.
			name: "LayoutCall Form field (page template)",
			rawData: map[string]any{
				"LayoutCall": map[string]any{
					"Form": "Atlas_Core.Phone_Default",
				},
			},
			want: "Atlas_Core.Phone_Default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractLayoutRef(tt.rawData); got != tt.want {
				t.Errorf("extractLayoutRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractPageWidgets(t *testing.T) {
	t.Run("no FormCall", func(t *testing.T) {
		got := extractPageWidgets(map[string]any{}, "container-1")
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("no Arguments", func(t *testing.T) {
		got := extractPageWidgets(map[string]any{
			"FormCall": map[string]any{},
		}, "container-1")
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("extracts widgets from arguments", func(t *testing.T) {
		rawData := map[string]any{
			"FormCall": map[string]any{
				"Arguments": []any{
					int32(0), // BSON type indicator
					map[string]any{
						"Widgets": []any{
							int32(0),
							map[string]any{
								"$ID":   "widget-1",
								"Name":  "textBox1",
								"$Type": "Forms$TextBox",
							},
						},
					},
				},
			},
		}
		got := extractPageWidgets(rawData, "container-1")
		if len(got) != 1 {
			t.Fatalf("expected 1 widget, got %d", len(got))
		}
		if got[0].Name != "textBox1" {
			t.Errorf("widget name = %q, want %q", got[0].Name, "textBox1")
		}
		if got[0].WidgetType != "Forms$TextBox" {
			t.Errorf("widget type = %q, want %q", got[0].WidgetType, "Forms$TextBox")
		}
	})
}

func TestExtractWidgetsRecursive(t *testing.T) {
	t.Run("simple widget", func(t *testing.T) {
		w := map[string]any{
			"$ID":   "w1",
			"Name":  "myWidget",
			"$Type": "Forms$TextBox",
		}
		got := extractWidgetsRecursive(w)
		if len(got) != 1 {
			t.Fatalf("expected 1 widget, got %d", len(got))
		}
		if got[0].ID != "w1" || got[0].Name != "myWidget" {
			t.Errorf("unexpected widget: %+v", got[0])
		}
	})

	t.Run("indexes a user DivContainer and its children", func(t *testing.T) {
		// A named/plain DivContainer is a real `container` widget (styleable,
		// clickable) — it must appear in the catalog alongside its children.
		w := map[string]any{
			"$ID":   "div1",
			"Name":  "divContainer",
			"$Type": "Forms$DivContainer",
			"Widgets": []any{
				int32(0),
				map[string]any{
					"$ID":   "child1",
					"Name":  "childWidget",
					"$Type": "Forms$Button",
				},
			},
		}
		got := extractWidgetsRecursive(w)
		if len(got) != 2 {
			t.Fatalf("expected 2 widgets (container + child), got %d", len(got))
		}
		if got[0].ID != "div1" || got[0].WidgetType != "Forms$DivContainer" {
			t.Errorf("expected the container first, got %+v", got[0])
		}
		if got[1].ID != "child1" {
			t.Errorf("expected child1 second, got %q", got[1].ID)
		}
	})

	t.Run("skips the transparent conditionalVisibilityWidget wrapper", func(t *testing.T) {
		// The synthetic layout placeholder (DivContainer named
		// conditionalVisibilityWidget*, no styling) is omitted; children stay.
		w := map[string]any{
			"$ID":   "div2",
			"Name":  "conditionalVisibilityWidget1",
			"$Type": "Pages$DivContainer",
			"Widgets": []any{
				int32(0),
				map[string]any{
					"$ID":   "child2",
					"Name":  "childWidget2",
					"$Type": "Pages$Button",
				},
			},
		}
		got := extractWidgetsRecursive(w)
		if len(got) != 1 {
			t.Fatalf("expected 1 widget (child only), got %d", len(got))
		}
		if got[0].ID != "child2" {
			t.Errorf("expected child2, got %q", got[0].ID)
		}
	})

	t.Run("indexes a styled conditionalVisibilityWidget (not transparent)", func(t *testing.T) {
		// If a wrapper-named container carries styling it is a real user widget.
		w := map[string]any{
			"$ID":        "div3",
			"Name":       "conditionalVisibilityWidget9",
			"$Type":      "Forms$DivContainer",
			"Appearance": map[string]any{"Class": "card"},
		}
		got := extractWidgetsRecursive(w)
		if len(got) != 1 || got[0].ID != "div3" {
			t.Fatalf("expected the styled container to be indexed, got %+v", got)
		}
	})

	t.Run("CustomWidget resolves WidgetId", func(t *testing.T) {
		w := map[string]any{
			"$ID":   "cw1",
			"Name":  "customWidget",
			"$Type": "CustomWidgets$CustomWidget",
			"Type": map[string]any{
				"WidgetId": "com.mendix.widget.MyCustom",
			},
		}
		got := extractWidgetsRecursive(w)
		if len(got) != 1 {
			t.Fatalf("expected 1 widget, got %d", len(got))
		}
		if got[0].WidgetType != "com.mendix.widget.MyCustom" {
			t.Errorf("WidgetType = %q, want %q", got[0].WidgetType, "com.mendix.widget.MyCustom")
		}
	})

	t.Run("extracts attribute reference", func(t *testing.T) {
		w := map[string]any{
			"$ID":   "w1",
			"Name":  "inputWidget",
			"$Type": "Forms$TextBox",
			"AttributeRef": map[string]any{
				"Attribute": "Module.Entity.Name",
			},
		}
		got := extractWidgetsRecursive(w)
		if len(got) != 1 {
			t.Fatalf("expected 1 widget, got %d", len(got))
		}
		if got[0].AttributeRef != "Module.Entity.Name" {
			t.Errorf("AttributeRef = %q, want %q", got[0].AttributeRef, "Module.Entity.Name")
		}
	})

	t.Run("recurses into LayoutGrid rows/columns", func(t *testing.T) {
		w := map[string]any{
			"$ID":   "grid1",
			"Name":  "layoutGrid",
			"$Type": "Forms$LayoutGrid",
			"Rows": []any{
				int32(0),
				map[string]any{
					"Columns": []any{
						int32(0),
						map[string]any{
							"Widgets": []any{
								int32(0),
								map[string]any{
									"$ID":   "nested1",
									"Name":  "nestedWidget",
									"$Type": "Forms$Text",
								},
							},
						},
					},
				},
			},
		}
		got := extractWidgetsRecursive(w)
		// grid1 itself + nested1
		if len(got) != 2 {
			t.Fatalf("expected 2 widgets, got %d", len(got))
		}
	})
}

func TestExtractSnippetWidgets(t *testing.T) {
	t.Run("Widgets plural format", func(t *testing.T) {
		rawData := map[string]any{
			"Widgets": []any{
				int32(0),
				map[string]any{
					"$ID":   "sw1",
					"Name":  "snippetWidget",
					"$Type": "Forms$TextBox",
				},
			},
		}
		got := extractSnippetWidgets(rawData)
		if len(got) != 1 {
			t.Fatalf("expected 1 widget, got %d", len(got))
		}
	})

	t.Run("Widget singular container format", func(t *testing.T) {
		rawData := map[string]any{
			"Widget": map[string]any{
				"Widgets": []any{
					int32(0),
					map[string]any{
						"$ID":   "sw1",
						"Name":  "innerWidget",
						"$Type": "Forms$Button",
					},
				},
			},
		}
		got := extractSnippetWidgets(rawData)
		if len(got) != 1 {
			t.Fatalf("expected 1 widget, got %d", len(got))
		}
	})

	t.Run("nil when no widgets", func(t *testing.T) {
		got := extractSnippetWidgets(map[string]any{})
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestGetBsonArrayElements(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want int // expected length, -1 for nil
	}{
		{"nil input", nil, -1},
		{"int32 type indicator", []any{int32(0), "a", "b"}, 2},
		{"int type indicator", []any{int(0), "a", "b"}, 2},
		{"no type indicator", []any{"a", "b"}, 2},
		{"bson.A with indicator", bson.A{int32(0), "a"}, 1},
		{"empty array", []any{}, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBsonArrayElements(tt.v)
			if tt.want == -1 {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
			} else if len(got) != tt.want {
				t.Errorf("len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestToBsonArray(t *testing.T) {
	t.Run("[]any passthrough", func(t *testing.T) {
		input := []any{"a", "b"}
		got := toBsonArray(input)
		if len(got) != 2 {
			t.Errorf("expected 2, got %d", len(got))
		}
	})

	t.Run("bson.A converted", func(t *testing.T) {
		input := bson.A{"x", "y"}
		got := toBsonArray(input)
		if len(got) != 2 {
			t.Errorf("expected 2, got %d", len(got))
		}
	})

	t.Run("unsupported type returns nil", func(t *testing.T) {
		got := toBsonArray("not an array")
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestExtractString(t *testing.T) {
	if got := extractString("hello"); got != "hello" {
		t.Errorf("expected hello, got %q", got)
	}
	if got := extractString(42); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := extractString(nil); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractBsonID(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"nil", nil, ""},
		{"string", "my-id", "my-id"},
		{"binary map with base64 GUID", map[string]any{"Data": "AQIDBAUGBwgJCgsMDQ4PEA=="}, "04030201-0605-0807-090a-0b0c0d0e0f10"},
		{"bson.Binary", bson.Binary{Data: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}}, "04030201-0605-0807-090a-0b0c0d0e0f10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractBsonID(tt.v); got != tt.want {
				t.Errorf("extractBsonID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeBase64GUID(t *testing.T) {
	t.Run("short data returned as-is", func(t *testing.T) {
		encoded := "AQIDBAUG" // only 6 bytes — too short for GUID
		got := decodeBase64GUID(encoded)
		if got != encoded {
			t.Errorf("expected passthrough for short data, got %q", got)
		}
	})

	t.Run("valid 16-byte GUID", func(t *testing.T) {
		// base64 of bytes 0x01..0x10
		encoded := "AQIDBAUGBwgJCgsMDQ4PEA=="
		got := decodeBase64GUID(encoded)
		want := "04030201-0605-0807-090a-0b0c0d0e0f10"
		if got != want {
			t.Errorf("decodeBase64GUID() = %q, want %q", got, want)
		}
	})

	t.Run("invalid base64", func(t *testing.T) {
		encoded := "not-valid-base64!!!"
		got := decodeBase64GUID(encoded)
		if got != encoded {
			t.Errorf("expected passthrough for invalid base64, got %q", got)
		}
	})
}

func TestExtractBinaryID(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"string", "my-id", "my-id"},
		{"bytes 16", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}, "04030201-0605-0807-090a-0b0c0d0e0f10"},
		{"bson.Binary", bson.Binary{Data: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}}, "04030201-0605-0807-090a-0b0c0d0e0f10"},
		{"nil", nil, ""},
		{"unsupported", 42, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractBinaryID(tt.v); got != tt.want {
				t.Errorf("extractBinaryID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatGUID(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "standard 16-byte GUID",
			data: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			want: "04030201-0605-0807-090a-0b0c0d0e0f10",
		},
		{
			name: "all zeros",
			data: make([]byte, 16),
			want: "00000000-0000-0000-0000-000000000000",
		},
		{
			name: "short data — returns raw string passthrough",
			data: []byte{0x41, 0x42},
			want: "AB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatGUID(tt.data); got != tt.want {
				t.Errorf("formatGUID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBytesToHex(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"zero byte", []byte{0x00}, "00"},
		{"max byte", []byte{0xff}, "ff"},
		{"two bytes", []byte{0x0a, 0xbc}, "0abc"},
		{"empty", []byte{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bytesToHex(tt.data); got != tt.want {
				t.Errorf("bytesToHex(%v) = %q, want %q", tt.data, got, tt.want)
			}
		})
	}
}

// upstream #773: a widget action that OPENS a page created no reference at all,
// so a page reachable only from a button reported "(no callers found)" — a false
// negative that reads as "safe to delete".
//
// The cause was one missing key. scanWidgetOwnRefs collected Entity, Microflow
// and Nanoflow but not `Form`, which is where a page reference lives — "Form"
// being Mendix's original word for a page, the same rename behind
// ShowFormAction/CloseFormAction.
func TestScanWidgetOwnRefs_PageReference(t *testing.T) {
	// An action button that opens a page: `show_page Module.Target`.
	showPage := map[string]any{
		"$Type": "Forms$ActionButton",
		"Action": map[string]any{
			"$Type":        "Forms$ShowPageClientAction",
			"PageSettings": map[string]any{"$Type": "Forms$PageSettings", "Form": "Sales.OrderDetail"},
		},
	}
	if _, _, _, page, _ := scanWidgetOwnRefs(showPage); page != "Sales.OrderDetail" {
		t.Errorf("page = %q, want Sales.OrderDetail", page)
	}

	// The compound the issue reported — "create object … then open page" — is ONE
	// action carrying BOTH an entity and a page. Collecting only the entity (which
	// is what happened) still leaves the page with no inbound reference.
	createThenShow := map[string]any{
		"$Type": "Forms$ActionButton",
		"Action": map[string]any{
			"$Type":        "Forms$CreateObjectClientAction",
			"Entity":       "Sales.Order",
			"PageSettings": map[string]any{"$Type": "Forms$PageSettings", "Form": "Sales.OrderDetail"},
		},
	}
	ent, _, _, page, _ := scanWidgetOwnRefs(createThenShow)
	if ent != "Sales.Order" {
		t.Errorf("entity = %q, want Sales.Order", ent)
	}
	if page != "Sales.OrderDetail" {
		t.Errorf("page = %q, want Sales.OrderDetail — the page half of "+
			"'create object … then open page' is the reference the issue reported missing", page)
	}

	// A child widget's page must not be attributed to its parent, same as the
	// existing microflow rule.
	container := map[string]any{
		"$Type": "Forms$DivContainer",
		"Widgets": []any{
			map[string]any{
				"$Type":  "Forms$ActionButton",
				"Action": map[string]any{"PageSettings": map[string]any{"Form": "Sales.ChildPage"}},
			},
		},
	}
	if _, _, _, page, _ := scanWidgetOwnRefs(container); page != "" {
		t.Errorf("page = %q, want empty — a child button's page must not leak to the container", page)
	}

	// A widget with no action references no page.
	if _, _, _, page, _ := scanWidgetOwnRefs(map[string]any{"$Type": "Forms$Label"}); page != "" {
		t.Errorf("page = %q, want empty", page)
	}
}
