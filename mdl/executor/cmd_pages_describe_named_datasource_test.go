// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func buildMultiSourceWidget() map[string]any {
	propertyType := func(id, key string) map[string]any {
		return map[string]any{"$ID": id, "PropertyKey": key}
	}
	property := func(id, flow string) map[string]any {
		return map[string]any{
			"TypePointer": id,
			"Value": map[string]any{"DataSource": map[string]any{
				"$Type":     "Forms$MicroflowSource",
				"Microflow": flow,
			}},
		}
	}
	return map[string]any{
		"Type": map[string]any{
			"WidgetId": "com.example.multisource.MultiSource",
			"ObjectType": map[string]any{"PropertyTypes": []any{
				propertyType("primary-id", "primarySource"),
				propertyType("secondary-id", "secondarySource"),
				propertyType("summary-id", "summarySource"),
			}},
		},
		"Object": map[string]any{"Properties": []any{
			property("primary-id", "Demo.DS_Primary"),
			property("secondary-id", "Demo.DS_Secondary"),
			property("summary-id", "Demo.DS_Summary"),
		}},
	}
}

func TestNamedCustomWidgetDataSourcesPreservePropertyKeys(t *testing.T) {
	sources := namedCustomWidgetDataSources(buildMultiSourceWidget())
	if len(sources) != 3 {
		t.Fatalf("got %d sources, want 3: %#v", len(sources), sources)
	}
	for i, want := range []string{"primarySource", "secondarySource", "summarySource"} {
		if sources[i].Key != want {
			t.Errorf("source %d key = %q, want %q", i, sources[i].Key, want)
		}
	}
}

func TestDescribeEmitsEveryNamedCustomWidgetDataSource(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}

	outputWidgetMDLV3(ctx, rawWidget{
		Type:             "CustomWidgets$CustomWidget",
		RenderMode:       "multisource",
		Name:             "dashboard",
		WidgetID:         "com.example.multisource.MultiSource",
		NamedDataSources: namedCustomWidgetDataSources(buildMultiSourceWidget()),
	}, 0)

	out := buf.String()
	for _, want := range []string{
		"primarySource: microflow Demo.DS_Primary",
		"secondarySource: microflow Demo.DS_Secondary",
		"summarySource: microflow Demo.DS_Summary",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("description missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "DataSource:") {
		t.Errorf("multi-source widget was collapsed to generic DataSource:\n%s", out)
	}

	page := "create page Demo.Showcase (Title: 'Demo') {\n" + out + "}\n"
	if _, errs := visitor.Build(page); len(errs) > 0 {
		t.Fatalf("named datasource DESCRIBE output is not valid MDL: %v\n%s", errs[0], page)
	}
}
