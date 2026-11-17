// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"strings"
	"testing"
)

// ako/mxcli#2310: a Java action whose body calls Core.getConfiguration() but
// states no `import com.mendix.core.Core;` was written without it, and passed
// check, exec and docker check (mx check does not compile Java); only the real
// build failed: "cannot find symbol: variable Core". Core is the Mendix runtime
// API almost every action body reaches for, so the generated file imports it
// when the code uses it. mxbuild keeps an import it finds (it never drops one),
// so its regeneration keeps this one too.

func TestGenerateSource_ImportsCoreWhenTheCodeUsesIt_Issue2310(t *testing.T) {
	src := GenerateSource("Helpdesk", "RunSparql",
		"File resources = Core.getConfiguration().getResourcesPath();\nreturn true;",
		nil, &BooleanType{}, nil, "", "")
	if !strings.Contains(src, "import com.mendix.core.Core;") {
		t.Errorf("generated source lacks the Core import:\n%s", src)
	}
}

// The extra code section counts too.
func TestGenerateSource_ImportsCoreForExtraCode_Issue2310(t *testing.T) {
	src := GenerateSource("M", "A", "return true;", nil, &BooleanType{}, nil,
		"private static void log() { Core.getLogger(\"M\").info(\"x\"); }", "")
	if !strings.Contains(src, "import com.mendix.core.Core;") {
		t.Errorf("generated source lacks the Core import for extra code:\n%s", src)
	}
}

// CONTROLS: no import when the code does not use Core — a string or a comment
// naming it, a longer identifier — or when it imports a Core of its own.
func TestGenerateSource_NoCoreImportOtherwise_Issue2310(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		imports    []string
	}{
		{"no use", "return true;", nil},
		{"in a string", "String s = \"Core.x\";\nreturn true;", nil},
		{"in a comment", "// Core.getConfiguration()\nreturn true;", nil},
		{"longer name", "MyCore.run();\nreturn true;", nil},
		{"own Core", "Core.run();\nreturn true;", []string{"import my.pkg.Core;"}},
		{"wildcard", "Core.run();\nreturn true;", []string{"import com.mendix.core.*;"}},
	} {
		src := GenerateSource("M", "A", tc.code, nil, &BooleanType{}, tc.imports, "", "")
		if strings.Contains(src, "import com.mendix.core.Core;") {
			t.Errorf("%s: Core imported:\n%s", tc.name, src)
		}
	}
}
