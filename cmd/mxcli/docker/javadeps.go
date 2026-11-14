// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// javadeps.go resolves a project's managed Java (JAR) dependencies.
//
// mxcli-formula1 findings #12: `ALTER MODULE X ADD JAR DEPENDENCY (…)` writes the
// coordinate to the model and `list jar dependencies` reports it — but the jar
// never reaches the classpath, and the only symptom is a runtime SQLException
// about a missing driver, long after the model looked right.
//
// The cause is not a bad write. Measured on 11.12.1: a full `mxbuild
// --target=deploy` produces a `deployment/build.gradle` with no dependencies
// block and downloads nothing, while `mx sync-java-dependencies <project.mpr>`
// fetches the jar into `vendorlib/`. Resolution is a separate step that Studio
// Pro runs for you when you edit Module Settings, and that nothing was running
// headless.

// SyncJavaDependencies runs `mx sync-java-dependencies` against projectPath,
// which downloads every declared managed dependency into the project's
// vendorlib/. It needs network access (Maven) and the mx binary.
//
// mxPath may be empty, in which case mx is resolved from the version's cache.
func SyncJavaDependencies(projectPath, mxPath, version string, w io.Writer) error {
	if mxPath == "" {
		mxPath = CachedMxPath(version)
	}
	if mxPath == "" {
		// No mx for this exact version. Fall back to any available one — a
		// newer mx reads an older project fine — but say so, because a
		// version mismatch is otherwise invisible until mx complains about
		// the mpr format and the message reads as a corrupt project.
		if resolved, err := ResolveMxForVersion("", version); err == nil && resolved != "" {
			mxPath = resolved
			if w != nil {
				fmt.Fprintf(w, "  Note: no mx for Mendix %s; using %s. Run 'mxcli setup mxbuild --version %s' if this misbehaves.\n",
					version, mxPath, version)
			}
		}
	}
	if mxPath == "" {
		return fmt.Errorf("mx not found for Mendix %s (needed to resolve managed Java dependencies); run 'mxcli setup mxbuild --version %s'", version, version)
	}

	out, err := syncJavaDepsOnce(mxPath, projectPath, w)
	// "Failed to find some hosts" is mx's word for a repository it could not
	// reach. Measured in ako/mxcli#2028 while Maven Central answered 429: the
	// first sync failed with it and a retry a moment later worked. Once, so a
	// network that is really down still fails promptly.
	if err != nil && strings.Contains(out, unreachableHostsMarker) {
		if w != nil {
			fmt.Fprintf(w, "  mx could not reach the Maven repository; retrying in %s...\n", syncJavaDepsRetryDelay)
		}
		time.Sleep(syncJavaDepsRetryDelay)
		out, err = syncJavaDepsOnce(mxPath, projectPath, w)
	}
	if err != nil {
		var werr *WatchdogError
		if errors.As(err, &werr) {
			return err // already names the step, the cause and the last output
		}
		return fmt.Errorf("mx sync-java-dependencies failed: %w\n%s", err, lastLines(out, 20))
	}
	if w != nil {
		if s := strings.TrimSpace(out); s != "" {
			fmt.Fprintln(w, "  "+strings.ReplaceAll(s, "\n", "\n  "))
		}
	}
	return nil
}

// unreachableHostsMarker is how mx reports a Maven repository it could not
// reach: "Failed to find some hosts. Either the network is down or the custom
// repository configuration contains an unknown host name."
const unreachableHostsMarker = "Failed to find some hosts"

// syncJavaDepsRetryDelay is the pause before retrying an unreachable-hosts
// failure. A variable so a test need not wait it out.
var syncJavaDepsRetryDelay = 10 * time.Second

// syncJavaDepsOnce runs one `mx sync-java-dependencies` and returns its
// combined output.
func syncJavaDepsOnce(mxPath, projectPath string, w io.Writer) (string, error) {
	cmd := syncJavaDepsCommand(mxPath, projectPath)
	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	// mx runs Gradle; a daemon it starts keeps the pipe open after mx exits
	// (#1396). mx's own exit status is the result — ErrWaitDelay only says the
	// daemon outlived it.
	cmd.WaitDelay = orphanPipeWaitDelay
	// Bounded, and not silent: the step used to wait for as long as Gradle did,
	// which behind a stale daemon was ten minutes and more with no output at
	// all (ako/mxcli#1340). The process tree is killed at the bound.
	err := runWithWatchdog(cmd, syncJavaDepsLimits(w), "")
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	return out.String(), err
}

// DefaultSyncJavaDepsTimeout bounds one `mx sync-java-dependencies`. A healthy
// sync takes seconds (6-12s measured in #1340, downloads included); the bound
// is generous because killing a slow download costs more than a late kill.
const DefaultSyncJavaDepsTimeout = 10 * time.Minute

// syncJavaDepsTimeoutEnv raises (or, with 0, disables) the sync's overall bound.
const syncJavaDepsTimeoutEnv = "MXCLI_SYNC_JAVA_DEPS_TIMEOUT"

// syncJavaDepsProgressEvery is how often a sync still running says so. A
// variable so a test need not wait it out.
var syncJavaDepsProgressEvery = 30 * time.Second

// staleGradleDaemonHint is the likely cause of a sync that never finishes.
const staleGradleDaemonHint = "Likely cause: a Gradle daemon left running by an earlier mx or mxbuild run, which this\n" +
	"sync attached to (ako/mxcli#1340). End the leftover Java processes whose command line\n" +
	"contains GradleDaemon (Task Manager on Windows; `pkill -f GradleDaemon` elsewhere),\n" +
	"then retry: mxcli sync-java-deps -p <app.mpr>"

// syncJavaDepsLimits are the sync's bounds: an overall timeout (no stall
// bound — Gradle can be quiet for a long download) and a progress line to w.
func syncJavaDepsLimits(w io.Writer) watchdogLimits {
	lim := watchdogLimits{
		Timeout:    durationEnv(syncJavaDepsTimeoutEnv, DefaultSyncJavaDepsTimeout),
		Step:       "mx sync-java-dependencies",
		TimeoutEnv: syncJavaDepsTimeoutEnv,
		Hint:       staleGradleDaemonHint,
	}
	if w != nil {
		lim.Progress = syncJavaDepsProgressEvery
		lim.OnProgress = func(elapsed time.Duration) {
			fmt.Fprintf(w, "  still resolving Java dependencies... (%s)\n", elapsed.Round(time.Second))
		}
	}
	return lim
}

// syncJavaDepsCommand builds the `mx sync-java-dependencies` invocation: a
// one-shot, so Gradle runs without a daemon (see prepareOneShotMxCommand).
func syncJavaDepsCommand(mxPath, projectPath string) *exec.Cmd {
	cmd := exec.Command(mxPath, "sync-java-dependencies", projectPath)
	cmd.Dir = filepath.Dir(projectPath)
	prepareOneShotMxCommand(cmd) // FreeType LD_PRELOAD workaround + no Gradle daemon
	return cmd
}

// UnvendoredJarDependencies returns the coordinates ("group:artifact:version")
// the sync would vendor that have no matching jar in the project's vendorlib/,
// so a caller can sync only when something is actually missing.
//
// Only the resolved version of each artifact counts (see
// resolveJarDependencies): checking every declaration made a coordinate that
// Gradle had dropped "missing" forever, with no sync able to supply it (#1450).
//
// The vendored file name is `<artifact>-<version>.jar`, which is what
// `mx sync-java-dependencies` writes.
func UnvendoredJarDependencies(projectDir string, deps []JarDependencyRef) []string {
	var missing []string
	for _, d := range resolveJarDependencies(deps) {
		if !jarVendored(filepath.Join(projectDir, "vendorlib"), d) {
			missing = append(missing, fmt.Sprintf("%s:%s:%s", d.Group, d.Artifact, d.Version))
		}
	}
	return missing
}

// jarVendored reports whether vendorlib holds the jar d resolves to. A plain
// version names one file. A Maven range ("[2.21.2,3)", ako/mxcli#2028) is
// resolved by Gradle to a concrete version, so any vendored version of the
// artifact inside the range satisfies it — looking for a file named after the
// range made a successful sync report the dependency "still missing".
func jarVendored(vendorlib string, d JarDependencyRef) bool {
	if !isJarVersionRange(d.Version) {
		if _, err := os.Stat(filepath.Join(vendorlib, fmt.Sprintf("%s-%s.jar", d.Artifact, d.Version))); err == nil {
			return true
		}
		// Gradle keeps the newest version a dependency graph asks for, and
		// another jar's transitive dependency can ask for a newer patch than
		// the one declared: a declared bcprov-jdk18on 1.81 is vendored as
		// 1.81.1, and reporting it "still missing" after a clean sync was
		// wrong (ako/mxcli#2248). A newer patch of the same major.minor
		// satisfies the declaration; another minor is a different jar.
		for _, v := range vendoredJarVersions(vendorlib, d.Artifact) {
			if newerPatch(v, d.Version) {
				return true
			}
		}
		return false
	}
	for _, v := range vendoredJarVersions(vendorlib, d.Artifact) {
		if jarVersionInRange(v, d.Version) {
			return true
		}
	}
	return false
}

// vendoredJarVersions returns the versions of artifact that vendorlib holds,
// read from the <artifact>-<version>.jar file names.
func vendoredJarVersions(vendorlib, artifact string) []string {
	entries, err := os.ReadDir(vendorlib)
	if err != nil {
		return nil
	}
	prefix := artifact + "-"
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".jar") {
			continue
		}
		v := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".jar")
		// A version starts with a digit; "jackson-databind-nullable-0.2.6.jar"
		// is another artifact sharing the name as a prefix.
		if v == "" || v[0] < '0' || v[0] > '9' {
			continue
		}
		out = append(out, v)
	}
	return out
}

// newerPatch reports whether vendored is a newer release of the same
// major.minor as declared: 1.81 -> 1.81.1, 3.18.0 -> 3.18.1, not 1.81 -> 1.82
// and never a qualifier such as 1.81-beta.
func newerPatch(vendored, declared string) bool {
	pv, pd := splitJarVersion(vendored), splitJarVersion(declared)
	if len(pd) < 2 || len(pv) < 2 {
		return false
	}
	for i := 0; i < 2; i++ {
		if !isJarVersionNumber(pd[i]) || compareJarVersionPart(pv[i], pd[i]) != 0 {
			return false
		}
	}
	return compareJarVersions(vendored, declared) > 0
}

// DescribeUnvendored adds to each missing coordinate the versions of that
// artifact vendorlib does hold, so a "still missing" names the cause:
// "org.x:y:1.2 (vendorlib has y-1.4.jar)".
func DescribeUnvendored(projectDir string, missing []string) []string {
	vendorlib := filepath.Join(projectDir, "vendorlib")
	out := make([]string, 0, len(missing))
	for _, m := range missing {
		parts := strings.Split(m, ":")
		if len(parts) < 3 {
			out = append(out, m)
			continue
		}
		vs := vendoredJarVersions(vendorlib, parts[1])
		if len(vs) == 0 {
			out = append(out, m)
			continue
		}
		jars := make([]string, len(vs))
		for i, v := range vs {
			jars[i] = parts[1] + "-" + v + ".jar"
		}
		out = append(out, fmt.Sprintf("%s (vendorlib has %s)", m, strings.Join(jars, ", ")))
	}
	return out
}

// trimJarZeros drops trailing ".0" parts, so a range bound "3" and the
// version "3.0.0" compare equal, as Maven compares them.
func trimJarZeros(v string) string {
	for strings.HasSuffix(v, ".0") {
		v = strings.TrimSuffix(v, ".0")
	}
	return v
}

// isJarVersionRange reports whether v is a Maven version range: "[1.0,2.0)",
// "(,1.5]", "[1.2]".
func isJarVersionRange(v string) bool {
	return strings.HasPrefix(v, "[") || strings.HasPrefix(v, "(")
}

// jarVersionInRange reports whether version v lies in the Maven range r. An
// empty bound is open; "[1.2]" is exactly 1.2.
func jarVersionInRange(v, r string) bool {
	if len(r) < 2 {
		return false
	}
	lowIncl, highIncl := r[0] == '[', r[len(r)-1] == ']'
	body := r[1 : len(r)-1]
	low, high, isPair := strings.Cut(body, ",")
	if !isPair {
		return compareJarVersions(trimJarZeros(v), trimJarZeros(strings.TrimSpace(body))) == 0
	}
	low, high = strings.TrimSpace(low), strings.TrimSpace(high)
	if low != "" {
		c := compareJarVersions(trimJarZeros(v), trimJarZeros(low))
		if c < 0 || (c == 0 && !lowIncl) {
			return false
		}
	}
	if high != "" {
		c := compareJarVersions(trimJarZeros(v), trimJarZeros(high))
		if c > 0 || (c == 0 && !highIncl) {
			return false
		}
	}
	return true
}

// JarDependencyRef is the coordinate of a managed Java dependency, decoupled
// from the model types so this package does not depend on the executor.
type JarDependencyRef struct {
	Group    string
	Artifact string
	Version  string
}

// resolveJarDependencies collapses the declarations to the one version of each
// group:artifact that `mx sync-java-dependencies` vendors, in first-declared
// order. Incomplete coordinates are dropped: they can be neither checked nor
// resolved.
//
// The sync runs Gradle over every module's declarations at once, and Gradle's
// conflict resolution keeps the newest version of an artifact declared more
// than once. Measured in #1450: ExcelImporter declaring commons-lang3 3.18.0 and
// CommunityCommons 3.20.0 leaves vendorlib/ holding commons-lang3-3.20.0.jar
// only, and a sync deletes a 3.18.0 jar placed there by hand.
func resolveJarDependencies(deps []JarDependencyRef) []JarDependencyRef {
	var out []JarDependencyRef
	index := map[string]int{} // group:artifact -> position in out
	for _, d := range deps {
		if d.Artifact == "" || d.Version == "" {
			continue
		}
		key := d.Group + ":" + d.Artifact
		if i, ok := index[key]; ok {
			if compareJarVersions(d.Version, out[i].Version) > 0 {
				out[i] = d
			}
			continue
		}
		index[key] = len(out)
		out = append(out, d)
	}
	return out
}

// SupersededJarDependencies lists the declarations the sync will not vendor
// because another module declares a newer version of the same artifact, as
// "group:artifact:version (resolved to newer)". They are not missing, but a
// module that needs exactly that version runs against the newer one, which is
// worth saying when nothing else in the output would.
func SupersededJarDependencies(deps []JarDependencyRef) []string {
	winner := map[string]string{}
	for _, d := range resolveJarDependencies(deps) {
		winner[d.Group+":"+d.Artifact] = d.Version
	}
	var out []string
	seen := map[string]bool{}
	for _, d := range deps {
		if d.Artifact == "" || d.Version == "" {
			continue
		}
		coord := fmt.Sprintf("%s:%s:%s", d.Group, d.Artifact, d.Version)
		w := winner[d.Group+":"+d.Artifact]
		if w == d.Version || seen[coord] {
			continue
		}
		seen[coord] = true
		out = append(out, fmt.Sprintf("%s (resolved to %s)", coord, w))
	}
	return out
}

// jarVersionSpecial ranks the qualifiers Gradle gives a meaning; any other
// non-numeric part ranks 0 and compares lexically against its peers.
var jarVersionSpecial = map[string]int{
	"dev": -1, "rc": 1, "snapshot": 2, "final": 3, "ga": 4, "release": 5, "sp": 6,
}

// compareJarVersions orders two versions the way Gradle's conflict resolution
// does: split on . - _ + and at digit/letter boundaries; numeric parts compare
// numerically and outrank non-numeric ones; a longer version wins when its
// extra part is numeric and loses when it is a qualifier (1.0-beta < 1.0).
// A string comparison gets the common case wrong: "3.9.0" > "3.20.0".
func compareJarVersions(a, b string) int {
	pa, pb := splitJarVersion(a), splitJarVersion(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if c := compareJarVersionPart(pa[i], pb[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(pa) > len(pb):
		if isJarVersionNumber(pa[len(pb)]) {
			return 1
		}
		return -1
	case len(pb) > len(pa):
		if isJarVersionNumber(pb[len(pa)]) {
			return -1
		}
		return 1
	}
	return 0
}

func compareJarVersionPart(x, y string) int {
	if x == y {
		return 0
	}
	xn, yn := isJarVersionNumber(x), isJarVersionNumber(y)
	switch {
	case xn && yn:
		x, y = strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
		if len(x) != len(y) {
			if len(x) < len(y) {
				return -1
			}
			return 1
		}
		return strings.Compare(x, y)
	case xn:
		return 1
	case yn:
		return -1
	}
	sx, xok := jarVersionSpecial[strings.ToLower(x)]
	sy, yok := jarVersionSpecial[strings.ToLower(y)]
	if xok || yok {
		if sx != sy {
			if sx < sy {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(x, y)
}

func splitJarVersion(v string) []string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	prevDigit := false
	for i, r := range v {
		switch {
		case r == '.' || r == '-' || r == '_' || r == '+':
			flush()
			continue
		case i > 0 && cur.Len() > 0 && (r >= '0' && r <= '9') != prevDigit:
			flush()
		}
		prevDigit = r >= '0' && r <= '9'
		cur.WriteRune(r)
	}
	flush()
	return parts
}

func isJarVersionNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
