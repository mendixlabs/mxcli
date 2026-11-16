// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeJavaHome makes a Java installation directory: bin/java(.exe), and
// bin/javac(.exe) when it is a JDK.
func fakeJavaHome(t *testing.T, root, name, exeSuffix string, jdk bool) string {
	t.Helper()
	home := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := []string{"java"}
	if jdk {
		tools = append(tools, "javac")
	}
	for _, tool := range tools {
		if err := os.WriteFile(filepath.Join(home, "bin", tool+exeSuffix), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// ako/mxcli#2186 (mendixlabs/mxcli#1839): with a JRE 21 and a JDK 21 side by
// side and JAVA_HOME unset, the JRE — first on PATH — was handed to MxBuild,
// which failed with "Java compiler is not available". A JRE reports the right
// `java -version`; what it lacks is bin/javac, so that is now required.
func TestResolveJDK_SkipsAJREWithoutACompiler_Issue2186(t *testing.T) {
	for _, suffix := range []string{".exe", ""} {
		root := t.TempDir()
		jre := fakeJavaHome(t, root, "jre-21.0.5.11-hotspot", suffix, false)
		jdk := fakeJavaHome(t, root, "jdk-21.0.2.13-hotspot", suffix, true)
		version21 := func(string) (string, error) { return `openjdk version "21.0.5" 2024-10-15`, nil }
		e := jdkEnv{
			goos:         "windows",
			getenv:       func(string) string { return "" },
			lookPath:     func(string) (string, error) { return filepath.Join(jre, "bin", "java"+suffix), nil },
			evalSymlinks: func(p string) (string, error) { return p, nil },
			isJDK:        func(home string, major int) bool { return jdkAt(home, major, version21) },
			glob:         filepath.Glob,
			searchPaths:  func(int) []string { return []string{filepath.Join(root, "jdk-21*")} },
			versionOf:    func(string) string { return "" },
		}
		got, err := resolveJDKWith(e, 21)
		if err != nil {
			t.Fatalf("suffix %q: %v", suffix, err)
		}
		if got != jdk {
			t.Errorf("suffix %q: resolved %s, want the JDK %s (the JRE has no javac)", suffix, got, jdk)
		}
	}
}

// JAVA_HOME on a JRE is skipped too, and when nothing else is found the error
// says that JAVA_HOME is a runtime without a compiler, not that its version is
// wrong — the version is right.
func TestResolveJDK_JavaHomeOnAJREIsNamed_Issue2186(t *testing.T) {
	root := t.TempDir()
	jre := fakeJavaHome(t, root, "jre-21", ".exe", false)
	version21 := func(string) (string, error) { return `openjdk version "21.0.5"`, nil }
	e := jdkEnv{
		goos:         "windows",
		getenv:       func(k string) string { return map[string]string{"JAVA_HOME": jre}[k] },
		lookPath:     func(string) (string, error) { return "", os.ErrNotExist },
		evalSymlinks: func(p string) (string, error) { return p, nil },
		isJDK:        func(home string, major int) bool { return jdkAt(home, major, version21) },
		glob:         filepath.Glob,
		searchPaths:  func(int) []string { return []string{filepath.Join(root, "jdk-21*")} },
		versionOf:    func(string) string { return `openjdk version "21.0.5"` },
	}
	_, err := resolveJDKWith(e, 21)
	if err == nil {
		t.Fatal("a JRE in JAVA_HOME was accepted as a JDK")
	}
	if !strings.Contains(err.Error(), "javac") {
		t.Errorf("the error does not say the JAVA_HOME installation lacks a compiler:\n%v", err)
	}
}

// CONTROL: a JDK of the wrong major is still refused on the version, and one of
// the right major with a compiler is accepted.
func TestJdkAt_VersionAndCompiler_Issue2186(t *testing.T) {
	root := t.TempDir()
	jdk := fakeJavaHome(t, root, "jdk", "", true)
	if !jdkAt(jdk, 21, func(string) (string, error) { return `openjdk version "21.0.2"`, nil }) {
		t.Error("a JDK 21 was refused")
	}
	if jdkAt(jdk, 21, func(string) (string, error) { return `openjdk version "17.0.9"`, nil }) {
		t.Error("a JDK 17 was accepted for 21")
	}
}
