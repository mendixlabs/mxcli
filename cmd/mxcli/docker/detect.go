// SPDX-License-Identifier: Apache-2.0

// Package docker implements Docker build and deployment support for Mendix projects.
//
// # Platform differences for Mendix tool resolution
//
// On Windows, Studio Pro installs mxbuild.exe, mx.exe, and the runtime under
// a Program Files directory (e.g., D:\Program Files\Mendix\11.6.4\).
// CDN downloads (mxbuild tar.gz) contain Linux ELF binaries that cannot
// execute on Windows, so Studio Pro installations MUST be preferred.
//
// On macOS, Studio Pro installs to "/Applications/Mendix Studio Pro X.Y.Z.app/".
// CDN downloads are Linux ELF binaries and also cannot run on macOS.
// Studio Pro installations are therefore preferred on macOS as well.
//
// On Linux (CI, devcontainers), Studio Pro is not available.
// CDN downloads are the primary source for mxbuild and runtime.
//
// Resolution priority (all platforms):
//  1. Explicit path (--mxbuild-path)
//  2. PATH lookup
//  3. OS-specific known locations (Studio Pro on Windows and macOS)
//  4. Cached CDN downloads (~/.mxcli/mxbuild/)
//
// Path discovery on Windows must NOT hardcode drive letters. Use environment
// variables (PROGRAMFILES, PROGRAMW6432, SystemDrive) to locate install dirs.
package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// macOSApplicationsDir is the root directory scanned for Studio Pro .app bundles
// on macOS. It is a package-level variable so tests can redirect it to a temp dir
// to avoid interference from real Studio Pro installations on the test host.
var macOSApplicationsDir = "/Applications"

// mxbuildBinaryName returns the platform-specific mxbuild binary name.
func mxbuildBinaryName() string {
	if runtime.GOOS == "windows" {
		return "mxbuild.exe"
	}
	return "mxbuild"
}

// mxbuildBinaryNames returns all candidate binary names for mxbuild.
// On Windows, the Linux binary ("mxbuild") may also be cached when downloaded
// for Docker, so both names must be checked.
func mxbuildBinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"mxbuild.exe", "mxbuild"}
	}
	return []string{"mxbuild"}
}

// findMxBuildInDir looks for the mxbuild binary inside a directory.
// Checks: dir/mxbuild, dir/modeler/mxbuild (Mendix installation layout).
func findMxBuildInDir(dir string) string {
	for _, bin := range mxbuildBinaryNames() {
		candidates := []string{
			filepath.Join(dir, bin),
			filepath.Join(dir, "modeler", bin),
		}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				return c
			}
		}
	}
	return ""
}

// resolveMxBuild finds the MxBuild executable.
// Priority: explicit path > PATH lookup > OS-specific known locations > cached downloads.
// On Windows, Studio Pro installations are checked before cached downloads because
// CDN downloads are Linux binaries that cannot run natively on Windows.
// The explicit path can be the binary itself or a directory containing it
// (e.g., a Mendix installation root with modeler/mxbuild inside).
func resolveMxBuild(explicitPath string, preferredVersion ...string) (string, error) {
	targetVersion := ""
	if len(preferredVersion) > 0 {
		targetVersion = preferredVersion[0]
	}

	if explicitPath != "" {
		info, err := os.Stat(explicitPath)
		if err != nil {
			return "", fmt.Errorf("mxbuild not found at %s: %w", explicitPath, err)
		}
		// If it's a directory, look for the binary inside it
		if info.IsDir() {
			if found := findMxBuildInDir(explicitPath); found != "" {
				return found, nil
			}
			return "", fmt.Errorf("mxbuild binary not found inside directory %s (looked for mxbuild and modeler/mxbuild)", explicitPath)
		}
		return explicitPath, nil
	}

	// Try PATH
	if p, err := exec.LookPath("mxbuild"); err == nil {
		return p, nil
	}

	// Try the exact Studio Pro installation for the project version first.
	// This keeps Docker builds on the same Mendix line as the project when
	// multiple installs are present locally.
	if targetVersion != "" {
		if studioProDir := ResolveStudioProDir(targetVersion); studioProDir != "" {
			if found := findMxBuildInDir(studioProDir); found != "" {
				return found, nil
			}
		}
	}

	// Try OS-specific known locations (Studio Pro on Windows) BEFORE cached downloads.
	// On Windows, CDN downloads are Linux binaries — Studio Pro's mxbuild.exe is preferred.
	if matches := globVersionedMatches(mxbuildSearchPaths()); len(matches) > 0 {
		if exact := exactVersionedPath(matches, targetVersion); exact != "" {
			return exact, nil
		}
		if newest := NewestVersionedPath(matches); newest != "" {
			return newest, nil
		}
	}

	if targetVersion != "" {
		if p := CachedMxBuildPath(targetVersion); p != "" {
			return p, nil
		}
	}

	// Try cached downloads (~/.mxcli/mxbuild/*/modeler/mxbuild)
	if p := AnyCachedMxBuildPath(); p != "" {
		return p, nil
	}

	return "", fmt.Errorf("mxbuild not found; install Mendix Studio Pro or specify --mxbuild-path")
}

// ResolveStudioProDir finds the Studio Pro installation directory for a specific
// Mendix version. Returns the installation root on Windows (e.g.,
// "D:\Program Files\Mendix\11.6.4") or the app Contents directory on macOS
// (e.g., "/Applications/Mendix Studio Pro 11.10.0.app/Contents").
// Returns empty string if not found or on Linux.
func ResolveStudioProDir(version string) string {
	switch runtime.GOOS {
	case "windows":
		for _, dir := range windowsProgramDirs() {
			candidate := filepath.Join(dir, "Mendix", version)
			if info, err := os.Stat(filepath.Join(candidate, "modeler", "mxbuild.exe")); err == nil && !info.IsDir() {
				return candidate
			}
		}
	case "darwin":
		return resolveStudioProDirMacOS(version)
	}
	return ""
}

// NativeMxBuildForSetup decides what `setup mxbuild` should do on the current
// host OS. The Mendix CDN only publishes Linux mxbuild archives (Linux ELF
// binaries), so on Windows and macOS a CDN download cannot execute natively —
// running it via dotnet fails with "Exec format error". On those platforms
// Studio Pro's bundled mxbuild is the correct binary.
//
// Returns:
//   - (path, "", nil)  — an installed Studio Pro mxbuild was found; use it, no download.
//   - ("", "", nil)    — the host is Linux (or the requested arch); a CDN download is appropriate.
//   - ("", guidance, error) — non-Linux host with no Studio Pro; the caller should
//     refuse to download and print `guidance` (unless the user forces it).
func NativeMxBuildForSetup(goos, version string) (path, guidance string, err error) {
	if goos == "linux" {
		return "", "", nil
	}

	if dir := ResolveStudioProDir(version); dir != "" {
		if found := findMxBuildInDir(dir); found != "" {
			return found, "", nil
		}
	}

	var hint string
	switch goos {
	case "windows":
		hint = fmt.Sprintf("Install Mendix Studio Pro %[1]s and use its bundled mxbuild, e.g.:\n"+
			"      \"C:\\Program Files\\Mendix\\%[1]s\\modeler\\mx.exe\" check app.mpr\n"+
			"  Or point mxcli at it explicitly with --mxbuild-path.\n"+
			"  (The Mendix CDN only publishes Linux mxbuild; downloading it on Windows would fail with \"Exec format error\".)\n"+
			"  To download the Linux binary anyway (e.g. to pre-cache it for a Docker/devcontainer build), re-run with --force.", version)
	case "darwin":
		hint = fmt.Sprintf("Install Mendix Studio Pro %[1]s and use its bundled mx, e.g.:\n"+
			"      \"/Applications/Mendix Studio Pro %[1]s.app/Contents/modeler/mx\" check app.mpr\n"+
			"  Or point mxcli at it explicitly with --mxbuild-path.\n"+
			"  (The Mendix CDN only publishes Linux mxbuild; downloading it on macOS would fail with \"Exec format error\".)\n"+
			"  To download the Linux binary anyway (e.g. to pre-cache it for a Docker build), re-run with --force.", version)
	default:
		hint = fmt.Sprintf("The Mendix CDN only publishes Linux mxbuild, which cannot run natively on %s.\n"+
			"  Install Mendix Studio Pro %s, or re-run with --force to download the Linux binary anyway.", goos, version)
	}
	return "", hint, fmt.Errorf("mxbuild from the Mendix CDN is a Linux binary and cannot run natively on %s", goos)
}

// resolveStudioProDirMacOS finds an installed Studio Pro on macOS that matches
// the requested version. App bundles are named "Mendix Studio Pro X.Y.Z*.app"
// where X.Y.Z is the base version (e.g., "11.10.0-rc.7 Beta" for 11.10.0).
// Returns the bundle's Contents directory, or "" if not found.
func resolveStudioProDirMacOS(version string) string {
	matches, _ := filepath.Glob(filepath.Join(macOSApplicationsDir, "Mendix Studio Pro *.app"))
	re := regexp.MustCompile(`^Mendix Studio Pro (\d+\.\d+\.\d+)`)
	for _, match := range matches {
		base := strings.TrimSuffix(filepath.Base(match), ".app")
		m := re.FindStringSubmatch(base)
		if m == nil || m[1] != version {
			continue
		}
		candidate := filepath.Join(match, "Contents")
		if _, err := os.Stat(filepath.Join(candidate, "modeler", "mx")); err == nil {
			return candidate
		}
	}
	return ""
}

// windowsProgramDirs returns candidate Program Files directories on Windows,
// derived from environment variables and the system drive letter.
func windowsProgramDirs() []string {
	seen := map[string]bool{}
	var dirs []string
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	for _, env := range []string{"PROGRAMFILES", "PROGRAMW6432", "PROGRAMFILES(X86)"} {
		add(os.Getenv(env))
	}
	// Fallback: derive from SystemDrive (e.g., "D:\Program Files").
	// SystemDrive returns "D:" without a trailing separator; filepath.Join
	// treats "D:" as a relative path, producing "D:Program Files" instead of
	// "D:\Program Files". Append the separator explicitly.
	if sysDrive := os.Getenv("SystemDrive"); sysDrive != "" {
		root := sysDrive + string(os.PathSeparator)
		add(filepath.Join(root, "Program Files"))
		add(filepath.Join(root, "Program Files (x86)"))
	}
	return dirs
}

// mendixSearchPaths returns OS-specific glob patterns for a Mendix binary
// (e.g., "mxbuild.exe", "mx.exe", "mxbuild", "mx") inside Studio Pro installations.
func mendixSearchPaths(binaryName string) []string {
	switch runtime.GOOS {
	case "windows":
		var paths []string
		for _, dir := range windowsProgramDirs() {
			paths = append(paths, filepath.Join(dir, "Mendix", "*", "modeler", binaryName))
		}
		return paths
	case "darwin":
		return []string{filepath.Join(macOSApplicationsDir, "Mendix Studio Pro *.app", "Contents", "modeler", binaryName)}
	default: // linux
		paths := []string{filepath.Join("/opt/mendix/*/modeler", binaryName)}
		if home, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(home, ".mendix/*/modeler", binaryName))
		}
		return paths
	}
}

// mxbuildSearchPaths returns OS-specific glob patterns for MxBuild.
func mxbuildSearchPaths() []string {
	return mendixSearchPaths(mxbuildBinaryName())
}

// resolveJDK finds a JDK of the requested major release.
// Priority: JAVA_HOME (verify version) > macOS java_home > java in PATH (verify version) > OS-specific known locations.
//
// The major comes from the PROJECT (Settings > Model > JavaVersion), not from a
// constant: Mendix 11.14's blank app asks for Java 25, matching Studio Pro's own
// support for it, while every version up to 11.13 asks for 21. Resolving 21
// unconditionally is what made an 11.14 project impossible to build or boot.
func resolveJDK(major int) (string, error) { return resolveJDKWith(hostJDKEnv(), major) }

// jdkEnv is everything resolveJDK asks of the machine, injectable so the macOS
// path is testable from a Linux runner — the /usr/bin/java stub that hung
// `mxcli new` exists on no machine CI runs on.
type jdkEnv struct {
	goos         string
	getenv       func(string) string
	macJavaHome  func(major int) (string, error) // /usr/libexec/java_home -v <major>
	lookPath     func(string) (string, error)
	evalSymlinks func(string) (string, error)
	isJDK        func(home string, major int) bool
	glob         func(string) ([]string, error)
	searchPaths  func(major int) []string
	versionOf    func(home string) string
}

func hostJDKEnv() jdkEnv {
	return jdkEnv{
		goos:         runtime.GOOS,
		getenv:       os.Getenv,
		macJavaHome:  resolveMacOSJavaHome,
		lookPath:     exec.LookPath,
		evalSymlinks: filepath.EvalSymlinks,
		isJDK:        isJDK,
		glob:         filepath.Glob,
		searchPaths:  jdkSearchPaths,
		versionOf:    javaVersionString,
	}
}

// isMacOSJavaStub reports whether home is what the PATH fallback derives from
// macOS's /usr/bin/java. That file is a launcher stub, not a symlink, so
// EvalSymlinks leaves it alone and the derived home is /usr. Worse, isJDK
// cannot tell: the stub forwards `-version` to the default JDK, so /usr
// "passes" for any major that happens to be installed, and MxBuild handed
// --java-home=/usr spins Gradle at 100% CPU indefinitely (ChipCoV8 #3).
func isMacOSJavaStub(home string) bool {
	return filepath.Clean(home) == "/usr"
}

func resolveJDKWith(e jdkEnv, major int) (string, error) {
	major = javaMajorOrDefault(major)
	darwin := e.goos == "darwin"
	// Try JAVA_HOME
	if javaHome := e.getenv("JAVA_HOME"); javaHome != "" {
		if !(darwin && isMacOSJavaStub(javaHome)) && e.isJDK(javaHome, major) {
			return javaHome, nil
		}
		// JAVA_HOME set but wrong version — continue searching
	}

	// On macOS, use /usr/libexec/java_home to find the real JDK path — for the
	// PROJECT's major, not a fixed 21 (a fixed 21 always failed the check below
	// for an 11.14+ project and fell through to the /usr/bin/java stub).
	var macErr error
	if darwin {
		javaHome, err := e.macJavaHome(major)
		switch {
		case err != nil:
			macErr = err
		case isMacOSJavaStub(javaHome):
			macErr = fmt.Errorf("java_home answered %s", javaHome)
		case e.isJDK(javaHome, major):
			return javaHome, nil
		default:
			macErr = fmt.Errorf("java_home answered %s, which is not a JDK %d", javaHome, major)
		}
	}

	// Try java in PATH
	if javaPath, err := e.lookPath("java"); err == nil {
		// Resolve symlinks to find the actual installation
		if resolved, err := e.evalSymlinks(javaPath); err == nil {
			javaPath = resolved
		}
		// java binary is typically at <jdk>/bin/java
		javaHome := filepath.Dir(filepath.Dir(javaPath))
		if !(darwin && isMacOSJavaStub(javaHome)) && e.isJDK(javaHome, major) {
			return javaHome, nil
		}
	}

	// Try OS-specific known locations
	searched := e.searchPaths(major)
	for _, pattern := range searched {
		matches, _ := e.glob(pattern)
		for _, m := range matches {
			if e.isJDK(m, major) {
				return m, nil
			}
		}
	}

	// Name what was searched. "JDK 21 not found" alone sent a Windows user
	// hunting through mxcli's source for the detection logic; the list makes it
	// obvious whether their JDK simply sits somewhere unlisted.
	msg := fmt.Sprintf("JDK %d not found", major)
	if jh := e.getenv("JAVA_HOME"); jh != "" {
		if javaTool(jh, "java") != "" && !hasJavaCompiler(jh) {
			msg += fmt.Sprintf("\n  JAVA_HOME is set to %s, a Java runtime (JRE) without bin/javac: MxBuild needs a JDK to compile Java actions", jh)
		} else {
			msg += fmt.Sprintf("\n  JAVA_HOME is set to %s but is not a JDK %d (java -version reports %s)", jh, major, e.versionOf(jh))
		}
	}
	if darwin {
		msg += fmt.Sprintf("\n  /usr/libexec/java_home -v %d found none", major)
		if macErr != nil {
			msg += fmt.Sprintf(" (%v)", firstLine(strings.TrimSpace(macErr.Error())))
		}
		msg += "; /usr/bin/java is the macOS launcher stub, not a JDK home, so it is not used."
	}
	if len(searched) > 0 {
		msg += "\n  Searched: " + strings.Join(searched, ", ")
	}
	msg += fmt.Sprintf("\n  Install Eclipse Temurin JDK %d, or set JAVA_HOME to one.", major)
	if darwin {
		msg += fmt.Sprintf("\n  On macOS: brew install --cask temurin@%d, then check with /usr/libexec/java_home -v %d"+
			"\n  (or export JAVA_HOME=$(/usr/libexec/java_home -v %d)).", major, major, major)
	}
	// Which Java a project needs is a MODEL setting, so a user staring at this
	// has a second way out that a bare "install a JDK" hides: build for a release
	// they already have. Only say so when the requirement is above the default:
	// below 11.14 the default is the only supported value, and a Mendix 9
	// project (JDK 17) has no JavaVersion setting at all, so the statement would
	// change nothing (mendixlabs/mxcli#1437).
	if major > DefaultJavaMajor {
		msg += fmt.Sprintf("\n  This is the project's own Settings > Model > JavaVersion. To build for a"+
			"\n  release you already have instead: mxcli -p <project>.mpr -c \"alter settings runtime ( JavaVersion: '%d' )\"", DefaultJavaMajor)
	}
	return "", fmt.Errorf("%s", msg)
}

// resolveMacOSJavaHome asks /usr/libexec/java_home for a JDK of the given major
// on macOS. java_home exits non-zero when none is installed.
func resolveMacOSJavaHome(major int) (string, error) {
	out, err := exec.Command("/usr/libexec/java_home", "-v", strconv.Itoa(major)).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// jdkSearchPaths returns OS-specific glob patterns for JDK installations.
func jdkSearchPaths(major int) []string { return jdkSearchPathsFor(runtime.GOOS, major) }

// jdkSearchPathsFor is jdkSearchPaths with the OS injected, so the Windows list
// is assertable from a Linux runner — nothing in CI executes Windows code, which
// is how the java.exe suffix stayed broken.
//
// Studio Pro does not ship a JDK of its own to point at: Mendix's install guide
// lists "Eclipse Temurin JDK 21 (x64 or ARM64)" as the prerequisite and installs
// it if absent, so the Temurin location below IS Studio Pro's JDK. (Its Gradle
// 8.5 does live with Studio Pro — "extracted to the parent directory of the
// folder where Studio Pro is installed (usually C:\Program Files\Mendix)" —
// but mxbuild invokes that itself; mxcli never calls gradle.)
func jdkSearchPathsFor(goos string, major int) []string {
	v := strconv.Itoa(javaMajorOrDefault(major))
	switch goos {
	case "windows":
		var paths []string
		for _, dir := range windowsProgramDirs() {
			paths = append(paths,
				filepath.Join(dir, "Eclipse Adoptium", "jdk-"+v+"*"),
				filepath.Join(dir, "Java", "jdk-"+v+"*"),
				filepath.Join(dir, "Microsoft", "jdk-"+v+"*"),
			)
		}
		// Per-user installs (winget and the Temurin MSI both offer one) land
		// outside Program Files, where nothing above would find them.
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			paths = append(paths,
				filepath.Join(local, "Programs", "Eclipse Adoptium", "jdk-"+v+"*"),
				filepath.Join(local, "Programs", "Microsoft", "jdk-"+v+"*"),
			)
		}
		return paths
	case "darwin":
		return []string{
			"/Library/Java/JavaVirtualMachines/temurin-" + v + "*/Contents/Home",
			"/Library/Java/JavaVirtualMachines/jdk-" + v + "*/Contents/Home",
		}
	default: // linux
		return []string{
			"/usr/lib/jvm/java-" + v + "-*",
			"/usr/lib/jvm/temurin-" + v + "-*",
		}
	}
}

// isJDK reports whether the given JAVA_HOME is a JDK of the requested major
// release. The version string is matched rather than parsed because `java
// -version` has never had a stable machine-readable form.
func isJDK(javaHome string, major int) bool {
	return jdkAt(javaHome, major, func(javaBin string) (string, error) {
		out, err := exec.Command(javaBin, "-version").CombinedOutput()
		return string(out), err
	})
}

// jdkAt is isJDK with `java -version` injected. A JDK is a java of the
// requested major AND a Java compiler: a JRE reports the same version, and
// MxBuild handed one fails compiling Java actions with "Java compiler is not
// available. Please check that <dir> contains a valid JDK installation". On
// Windows with a JRE 21 and a JDK 21 side by side and JAVA_HOME unset, the JRE
// came first on PATH and was chosen (mendixlabs/mxcli#1839, ako/mxcli#2186).
func jdkAt(javaHome string, major int, javaVersion func(javaBin string) (string, error)) bool {
	javaBin := javaTool(javaHome, "java")
	if javaBin == "" || !hasJavaCompiler(javaHome) {
		return false
	}
	out, err := javaVersion(javaBin)
	if err != nil {
		return false
	}
	return javaMajorRegex(major).MatchString(out)
}

// hasJavaCompiler reports whether a Java installation has bin/javac — what
// tells a JDK from a JRE.
func hasJavaCompiler(javaHome string) bool { return javaTool(javaHome, "javac") != "" }

// javaTool returns the path of bin/<tool> or bin/<tool>.exe in a Java
// installation, whichever exists, or "". Both spellings are looked for so the
// Windows layout is testable from a Linux runner.
func javaTool(javaHome, tool string) string {
	for _, name := range []string{tool, tool + ".exe"} {
		p := filepath.Join(javaHome, "bin", name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// javaMajorRegex matches the major release in `java -version` output, which
// prints `version "21.0.4"` or `version "25"` — so the major must be followed by
// a dot, whitespace or the closing quote, or 21 would also match 210.
func javaMajorRegex(major int) *regexp.Regexp {
	v := regexp.QuoteMeta(strconv.Itoa(javaMajorOrDefault(major)))
	return regexp.MustCompile(`version "` + v + `[\.\s"]`)
}

// javaVersionString runs java -version and returns the output for diagnostics.
func javaVersionString(javaHome string) string {
	javaBin := JavaExePath(javaHome)
	out, err := exec.Command(javaBin, "-version").CombinedOutput()
	if err != nil {
		return "(unknown)"
	}
	lines := strings.SplitN(string(out), "\n", 2)
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return "(unknown)"
}

// ResolveJDK reports the JAVA_HOME for a Java release, or an error naming what
// was searched. Exported so callers outside this package can ask whether the
// environment can build a given project before deciding what to do about it —
// `mxcli new` uses it to leave a project's Java version alone when a matching
// JDK exists.
func ResolveJDK(major int) (string, error) { return resolveJDK(major) }
