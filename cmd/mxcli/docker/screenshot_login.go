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

// screenshot_login.go logs into a secured Mendix app once and saves a Playwright
// storage state (cookies + local storage), so subsequent screenshots of pages
// behind login use `playwright screenshot --load-storage`. The `playwright` CLI
// has no scriptable login, so this drives a tiny headless script via the same
// Playwright install (resolved from the CLI's package dir — no hardcoded paths).
//
// The Mendix Atlas login page exposes stable selectors: #usernameInput,
// #passwordInput, #loginButton. If no login form appears (anonymous/public app),
// the script still saves storage and proceeds.

// resolvePlaywrightPkgDir returns the directory of the installed `playwright`
// package (the parent for requiring playwright-core), or "" if not found.
func resolvePlaywrightPkgDir() string {
	bin, err := exec.LookPath("playwright")
	if err != nil {
		return ""
	}
	real, err := filepath.EvalSymlinks(bin)
	if err != nil {
		real = bin
	}
	// real is <pkg>/cli.js; the package dir is its directory.
	dir := filepath.Dir(real)
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		return ""
	}
	return dir
}

// resolveNodeForScript returns a node binary to run the login script: the system
// node if on PATH, else mxbuild's bundled node resolved from mxbuildPath.
func resolveNodeForScript(mxbuildPath string) string {
	if p, err := exec.LookPath("node"); err == nil {
		return p
	}
	if mxbuildPath != "" {
		// The node binary alone, not resolveNodeTooling: that also requires
		// rollup-runner.mjs, which mxbuild 11.15 no longer ships (rspack only),
		// so on a machine whose newest cached mxbuild is 11.15 no node was found.
		if nodeBin := findNodeBinary(filepath.Join(filepath.Dir(mxbuildPath), "tools", "node")); nodeBin != "" {
			return nodeBin
		}
	}
	return ""
}

// loginScript is the headless login+save-storage script. It requires
// playwright-core from the resolved package dir (arg 1). Remaining args:
// appURL, username, password, storagePath.
const loginScript = `
const pkgDir = process.argv[2];
const [appURL, username, password, storagePath] = process.argv.slice(3);
// The playwright package beside the CLI on PATH when there is one; otherwise
// whatever NODE_PATH resolves (the devcontainer installs only @playwright/cli,
// whose bundled playwright-core is on the NODE_PATH mxcli builds).
const pw = (() => {
  if (pkgDir) { try { return require(require.resolve("playwright-core", { paths: [pkgDir] })); } catch (e) {} }
  try { return require("playwright"); } catch (e) { return require("playwright-core"); }
})();
const { chromium } = pw;
(async () => {
  const exe = process.env.MXCLI_CHROMIUM_EXE;
  const b = await chromium.launch(exe ? { executablePath: exe } : {});
  // Tell the runtime the request really is http when it is. On Mendix 10.24+
  // X-Forwarded-Proto takes precedence over ApplicationRootUrl, so this drops
  // the Secure attribute and the __Host- prefix from the session cookies. It
  // matters when the app boots with an https root URL (as it does under --hub)
  // and the browser reaches it over http from a NON-loopback host: 127.0.0.1 is
  // a trustworthy origin and keeps Secure cookies happily, but a container
  // hostname is not, and there the session cannot be held at all.
  //
  // Accurate rather than a workaround — the request is http — and a no-op when
  // the root URL is already http. Real users arrive over https through the hub
  // without this header and still get Secure cookies.
  const insecure = new URL(appURL).protocol === "http:";
  const ctx = await b.newContext(
    insecure ? { extraHTTPHeaders: { "X-Forwarded-Proto": "http" } } : {});
  const p = await ctx.newPage();
  await p.goto(appURL, { waitUntil: "load", timeout: 30000 });
  let sawForm = false;
  let failure = "";
  // An app that lets anonymous users in renders its home page at the root,
  // not the sign-in form; the form is still at /login.html.
  const hasForm = await p.waitForSelector("#usernameInput", { timeout: 8000 }).then(() => true, () => false);
  if (!hasForm) {
    await p.goto(new URL("/login.html", appURL).href, { waitUntil: "load", timeout: 30000 }).catch(() => {});
  }
  try {
    await p.waitForSelector("#usernameInput", { timeout: 8000 });
    sawForm = true;
    await p.fill("#usernameInput", username);
    await p.fill("#passwordInput", password);
    await Promise.all([
      p.waitForNavigation({ waitUntil: "load", timeout: 20000 }).catch(() => {}),
      p.click("#loginButton"),
    ]);
    await p.waitForTimeout(2500);
  } catch (e) {
    // No login form within the timeout: anonymous or already authenticated.
    process.stderr.write("login: no login form detected (" + e.message.split("\n")[0] + ")\n");
  }
  // Mendix answers a rejected sign-in by re-rendering the same form, so the
  // username field still being there is the signal that login did not complete.
  // Without this the script saved an anonymous session and every later
  // screenshot silently showed the login page instead of the requested page.
  if (sawForm && (await p.locator("#usernameInput").count()) > 0) {
    const alert = ((await p.locator(".alert").first().textContent().catch(() => "")) || "")
      .replace(/\s+/g, " ").trim().slice(0, 200);
    failure = alert || "still on the login page after submitting";
  }
  await ctx.storageState({ path: storagePath });
  await b.close();
  if (failure) {
    process.stderr.write("login: sign-in did not complete: " + failure + "\n");
    process.exit(2);
  }
})().catch((e) => { process.stderr.write(String(e) + "\n"); process.exit(1); });
`

// LoginOptions configures LoginAndSaveStorage.
type LoginOptions struct {
	AppURL      string
	Username    string
	Password    string
	StoragePath string // where to write the Playwright storage state JSON
	MxBuildPath string // fallback node source
	// RuntimeLogPath is consulted when a sign-in is rejected: the runtime
	// records why, and the page does not.
	RuntimeLogPath string
	Timeout        time.Duration // default 60s
}

// sessionCapMarker is what the unlicensed runtime logs when it refuses a sign-in
// because every session slot is taken. The browser only ever says "Sign in
// failed", which points at the credentials — so the cause is invisible unless
// someone thinks to open the runtime log.
const sessionCapMarker = "Maximum number of sessions exceeded"

// loginFailureHint returns an explanation to append to a failed sign-in, read
// from the tail of the runtime log. Empty when the log says nothing useful.
func loginFailureHint(runtimeLogPath string) string {
	if runtimeLogPath == "" || runtimeLogPath == "-" {
		return ""
	}
	tail, err := readLogTail(runtimeLogPath, 64*1024)
	if err != nil || !strings.Contains(tail, sessionCapMarker) {
		return ""
	}
	return "\nThe runtime log reports: " + sessionCapMarker + " — the unlicensed local\n" +
		"runtime allows 6 concurrent sessions, and the login page reports a 7th sign-in\n" +
		"(HTTP 560) as \"Sign in failed\" as if the password were wrong. Every sign-in\n" +
		"holds a slot until it signs out or expires. Free them all without a restart with\n" +
		"`mxcli run clear-sessions -p <app.mpr>` (signed-in browsers sign in again), or\n" +
		"restart `mxcli run --local`, which deletes old sessions as it starts. A script\n" +
		"that drives the app through a browser should sign out when it finishes."
}

// readLogTail reads at most the last n bytes of a file.
func readLogTail(path string, n int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if off := info.Size() - n; off > 0 {
		if _, err := f.Seek(off, io.SeekStart); err != nil {
			return "", err
		}
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// LoginAndSaveStorage logs into the app and writes a Playwright storage-state
// file to opts.StoragePath. That file is then passed to CaptureScreenshot via
// StoragePath (screenshot --load-storage) to shoot pages behind login.
func LoginAndSaveStorage(opts LoginOptions) error {
	if opts.StoragePath == "" {
		return fmt.Errorf("storage path is required")
	}
	// Empty pkgDir is fine: the script then resolves Playwright through the
	// NODE_PATH set below, as the page check does.
	pkgDir := resolvePlaywrightPkgDir()
	node := resolveNodeForScript(opts.MxBuildPath)
	if node == "" {
		return fmt.Errorf("node not found to run the login script")
	}
	if err := os.MkdirAll(filepath.Dir(opts.StoragePath), 0o755); err != nil {
		return err
	}

	// .cjs so Node treats it as CommonJS (the script uses require()).
	scriptPath := filepath.Join(filepath.Dir(opts.StoragePath), ".mxcli-login.cjs")
	if err := os.WriteFile(scriptPath, []byte(loginScript), 0o644); err != nil {
		return fmt.Errorf("writing login script: %w", err)
	}
	defer os.Remove(scriptPath)

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	cmd := exec.Command(node, scriptPath, pkgDir, opts.AppURL, opts.Username, opts.Password, opts.StoragePath)
	// PLAYWRIGHT_BROWSERS_PATH resolves Chromium; NODE_PATH resolves Playwright.
	cmd.Env = directRouteEnv(append(os.Environ(), "NODE_PATH="+playwrightNodePath(node)), opts.AppURL)
	if exe := headlessShellPath(); exe != "" {
		cmd.Env = append(cmd.Env, "MXCLI_CHROMIUM_EXE="+exe)
	}
	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching login script: %w", err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("login failed: %w\n%s%s", err, out.String(), loginFailureHint(opts.RuntimeLogPath))
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("login timed out after %s", timeout)
	}
	if _, err := os.Stat(opts.StoragePath); err != nil {
		return fmt.Errorf("login reported success but %s is missing:\n%s", opts.StoragePath, out.String())
	}
	return nil
}

// signOutScript signs a saved session out of the app the way the client's own
// sign-out does: mx.logout() posts {"action":"logout"} to /xas/ with the
// client's CSRF token. Measured on 11.13: the runtime's session count
// (M2EE runtime_statistics, user_sessions) drops by one. Args: appURL,
// storagePath. Exit 3 when there is no session left to sign out: a session
// the runtime has forgotten is answered 401 and sent to /login.html, which
// has no mx (measured on 11.15 after a restart, ~1.6s).
const signOutScript = `
const [appURL, storagePath] = process.argv.slice(2);
const pw = (() => { try { return require("playwright"); } catch (e) { return require("playwright-core"); } })();
const { chromium } = pw;
(async () => {
  const exe = process.env.MXCLI_CHROMIUM_EXE;
  const b = await chromium.launch(exe ? { executablePath: exe } : {});
  const ctx = await b.newContext({ storageState: storagePath });
  const p = await ctx.newPage();
  await p.goto(appURL, { waitUntil: "domcontentloaded", timeout: 30000 });
  await p.waitForSelector(".mx-page, #usernameInput", { timeout: 20000 }).catch(() => {});
  const client = await p.evaluate(() => !!(window.mx && typeof mx.logout === "function")).catch(() => false);
  if (!client) { await b.close(); process.exit(3); }
  const done = p.waitForResponse(r => r.request().method() === "POST" &&
    (r.request().postData() || "").includes('"logout"'), { timeout: 15000 });
  await p.evaluate(() => { mx.logout(); });
  const r = await done;
  await b.close();
  if (!r.ok()) { process.stderr.write("logout answered HTTP " + r.status() + "\n"); process.exit(2); }
})().catch((e) => { process.stderr.write(String(e).split("\n")[0] + "\n"); process.exit(1); });
`

// SignOutOptions configures SignOut.
type SignOutOptions struct {
	AppURL      string
	StoragePath string // the Playwright storage state holding the session
	MxBuildPath string // fallback node source
	Timeout     time.Duration
}

// ErrNoSession is SignOut's answer when the saved session is already gone —
// the runtime restarted or expired it — so there was nothing to sign out.
var ErrNoSession = errors.New("no live session to sign out")

// SignOut ends the session saved in opts.StoragePath, so it stops holding one
// of the unlicensed runtime's few session slots. It does not remove the file.
func SignOut(opts SignOutOptions) error {
	node := resolveNodeForScript(opts.MxBuildPath)
	if node == "" {
		return fmt.Errorf("node not found to run the sign-out script")
	}
	dir, err := os.MkdirTemp("", "mxcli-signout-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	scriptPath := filepath.Join(dir, "signout.cjs")
	if err := os.WriteFile(scriptPath, []byte(signOutScript), 0o600); err != nil {
		return err
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	cmd := exec.Command(node, scriptPath, opts.AppURL, opts.StoragePath)
	cmd.Env = directRouteEnv(append(os.Environ(), "NODE_PATH="+playwrightNodePath(node)), opts.AppURL)
	if exe := headlessShellPath(); exe != "" {
		cmd.Env = append(cmd.Env, "MXCLI_CHROMIUM_EXE="+exe)
	}
	out := &syncBuffer{}
	cmd.Stdout, cmd.Stderr = out, out
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching the sign-out script: %w", err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 3 {
			return ErrNoSession
		}
		if err != nil {
			return fmt.Errorf("%w: %s", err, clip(out.String(), 200))
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("timed out after %s", timeout)
	}
	return nil
}
