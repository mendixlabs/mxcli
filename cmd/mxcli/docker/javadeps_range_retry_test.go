// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ako/mxcli#2028: MCP Client 4.4.0 declares jackson-databind as the Maven range
// [2.21.2,3). `mx sync-java-dependencies` vendored jackson-databind-2.23.0.jar,
// but the check looked for a file named jackson-databind-[2.21.2,3).jar, so the
// sync reported "still missing" after succeeding and `run --local` re-synced on
// every boot. A range is satisfied by a vendored version inside it.
func TestUnvendoredJarDependencies_VersionRange(t *testing.T) {
	dep := []JarDependencyRef{{Group: "com.fasterxml.jackson.core", Artifact: "jackson-databind", Version: "[2.21.2,3)"}}
	for _, tc := range []struct {
		jar     string
		missing bool
	}{
		{"jackson-databind-2.23.0.jar", false},
		{"jackson-databind-2.21.2.jar", false}, // inclusive lower bound
		{"jackson-databind-2.20.0.jar", true},  // below
		{"jackson-databind-3.0.0.jar", true},   // exclusive upper bound
		// Another artifact sharing the name as a prefix is not this one.
		{"jackson-databind-nullable-0.2.6.jar", true},
	} {
		t.Run(tc.jar, func(t *testing.T) {
			dir := t.TempDir()
			writeVendoredJar(t, dir, tc.jar)
			got := UnvendoredJarDependencies(dir, dep)
			if (len(got) == 1) != tc.missing {
				t.Errorf("vendored %s: missing = %v, want %v", tc.jar, got, tc.missing)
			}
		})
	}
}

func TestJarVersionInRange(t *testing.T) {
	for _, tc := range []struct {
		v, r string
		want bool
	}{
		{"2.23.0", "[2.21.2,3)", true},
		{"3.0.0", "[2.21.2,3)", false},
		{"3.0.0", "[2.21.2,3]", true},
		{"1.0", "(1.0,2.0)", false},
		{"1.5", "(1.0,2.0)", true},
		{"9.9", "[1.0,)", true},
		{"0.9", "[1.0,)", false},
		{"1.2", "(,1.5]", true},
		{"1.2", "[1.2]", true},
		{"1.3", "[1.2]", false},
	} {
		if got := jarVersionInRange(tc.v, tc.r); got != tc.want {
			t.Errorf("jarVersionInRange(%q, %q) = %v, want %v", tc.v, tc.r, got, tc.want)
		}
	}
}

// ako/mxcli#2028: the first sync failed with mx's "Failed to find some hosts.
// Either the network is down or the custom repository configuration contains an
// unknown host name." while Maven Central was answering 429; a retry worked.
// The sync retries that failure once; any other failure is returned at once.
func TestSyncJavaDependencies_RetriesUnreachableHostsOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake mx is a shell script")
	}
	orig := syncJavaDepsRetryDelay
	syncJavaDepsRetryDelay = time.Millisecond
	t.Cleanup(func() { syncJavaDepsRetryDelay = orig })

	for _, tc := range []struct {
		name      string
		failures  int // how many runs fail before one succeeds
		message   string
		wantRuns  int
		wantError bool
	}{
		{"transient host failure is retried", 1, "Failed to find some hosts. Either the network is down", 2, false},
		{"a second host failure is returned", 2, "Failed to find some hosts. Either the network is down", 2, true},
		{"any other failure is not retried", 1, "Could not resolve com.example:nope:1.0", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			count := filepath.Join(dir, "runs")
			mx := filepath.Join(dir, "mx")
			script := "#!/bin/sh\nn=$(cat " + count + " 2>/dev/null || echo 0)\nn=$((n+1))\necho $n > " + count + "\n" +
				"if [ $n -le " + strconv.Itoa(tc.failures) + " ]; then echo '" + tc.message + "'; exit 1; fi\necho synced\n"
			if err := os.WriteFile(mx, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			err := SyncJavaDependencies(filepath.Join(dir, "App.mpr"), mx, "11.12.6", nil)
			if (err != nil) != tc.wantError {
				t.Fatalf("err = %v, want error %v", err, tc.wantError)
			}
			runs, _ := os.ReadFile(count)
			if got := strings.TrimSpace(string(runs)); got != strconv.Itoa(tc.wantRuns) {
				t.Errorf("mx ran %s time(s), want %d", got, tc.wantRuns)
			}
		})
	}
}
