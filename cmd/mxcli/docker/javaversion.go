// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultJavaMajor is the Java release assumed when a project does not say which
// one it wants, or cannot be read. Every Mendix 9/10/11 project up to 11.13 uses
// it, so it is also the answer for the overwhelming majority of projects.
const DefaultJavaMajor = 21

// ProjectJavaMajor reads the Java release a project is built for.
//
// Mendix 11.14 is the first version whose blank app asks for something other
// than 21: `mx create-project` writes 25, matching Studio Pro's own support for
// Java 25. Before this, mxcli resolved a JDK 21 unconditionally and refused any
// other JAVA_HOME, so an 11.14 project could not be built (`error: release
// version 25 not supported`) or booted (`UnsupportedClassVersionError … class
// file version 69.0` when compiled for 25 and launched on 21).
//
// The stored key is version-specific — JavaVersion "Java21" on 11.6,
// JavaMajorVersion "21" from 11.12 — which settingsoverlay already normalises,
// so both spellings arrive here as a bare number.
//
// A project that cannot be read, or whose value is not a number, reports
// DefaultJavaMajor and false. The caller then behaves exactly as mxcli did
// before: guessing a different JDK from an unreadable model would trade a clear
// failure for a confusing one.
func ProjectJavaMajor(projectPath string) (int, bool) {
	reader, err := openReadOnly(projectPath)
	if err != nil {
		return DefaultJavaMajor, false
	}
	defer func() { _ = reader.Disconnect() }()

	ps, err := reader.GetProjectSettings()
	if err != nil || ps == nil || ps.Model == nil {
		return DefaultJavaMajor, false
	}
	n, ok := ParseJavaMajor(ps.Model.JavaVersion)
	if !ok {
		return DefaultJavaMajor, false
	}
	return n, true
}

// ParseJavaMajor extracts the major release from either spelling Mendix has used
// for this setting: "21" and "Java21" both yield 21.
func ParseJavaMajor(v string) (int, bool) {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "Java")
	s = strings.TrimPrefix(s, "java")
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// javaMajorOrDefault normalises a possibly-unset major to something resolvable.
func javaMajorOrDefault(major int) int {
	if major <= 0 {
		return DefaultJavaMajor
	}
	return major
}

// describeJavaRequirement is the phrase used in errors and progress lines, so a
// user who has the wrong JDK sees which one the PROJECT asked for rather than a
// bare number they have to trace back to a model setting.
func describeJavaRequirement(major int, fromProject bool) string {
	if fromProject {
		return fmt.Sprintf("JDK %d (the project's JavaVersion)", major)
	}
	return fmt.Sprintf("JDK %d", major)
}
