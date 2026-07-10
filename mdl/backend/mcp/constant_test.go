// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestBuildConstantContent(t *testing.T) {
	c := &model.Constant{Name: "MaxRetries", Type: model.ConstantDataType{Kind: "Integer"}, DefaultValue: "5", ExposedToClient: true}
	m, err := buildConstantContent(c)
	if err != nil {
		t.Fatal(err)
	}
	if m["name"] != "MaxRetries" || m["type"] != "Integer" || m["defaultValue"] != "5" || m["exposedToClient"] != true {
		t.Fatalf("constant content: %+v", m)
	}
	// Date normalises to DateTime.
	d, _ := buildConstantContent(&model.Constant{Name: "D", Type: model.ConstantDataType{Kind: "Date"}})
	if d["type"] != "DateTime" {
		t.Fatalf("Date should map to DateTime: %+v", d)
	}
	// Types PED's constant constructor can't express are rejected, not coerced.
	for _, kind := range []string{"Long", "Enumeration", "Binary", "Object"} {
		if _, err := buildConstantContent(&model.Constant{Name: "X", Type: model.ConstantDataType{Kind: kind}}); err == nil {
			t.Errorf("constant type %q should be rejected", kind)
		}
	}
}
