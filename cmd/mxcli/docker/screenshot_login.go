// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		if nodeBin, _, err := resolveNodeTooling(mxbuildPath); err == nil {
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
const { chromium } = require(require.resolve("playwright-core", { paths: [pkgDir] }));
(async () => {
  const b = await chromium.launch();
  const ctx = await b.newContext();
  const p = await ctx.newPage();
  await p.goto(appURL, { waitUntil: "load", timeout: 30000 });
  try {
    await p.waitForSelector("#usernameInput", { timeout: 8000 });
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
  await ctx.storageState({ path: storagePath });
  await b.close();
})().catch((e) => { process.stderr.write(String(e) + "\n"); process.exit(1); });
`

// LoginOptions configures LoginAndSaveStorage.
type LoginOptions struct {
	AppURL      string
	Username    string
	Password    string
	StoragePath string        // where to write the Playwright storage state JSON
	MxBuildPath string        // fallback node source
	Timeout     time.Duration // default 60s
}

// LoginAndSaveStorage logs into the app and writes a Playwright storage-state
// file to opts.StoragePath. That file is then passed to CaptureScreenshot via
// StoragePath (screenshot --load-storage) to shoot pages behind login.
func LoginAndSaveStorage(opts LoginOptions) error {
	if opts.StoragePath == "" {
		return fmt.Errorf("storage path is required")
	}
	pkgDir := resolvePlaywrightPkgDir()
	if pkgDir == "" {
		return fmt.Errorf("playwright package not found (install with: npm i -g playwright)")
	}
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
	cmd.Env = os.Environ() // PLAYWRIGHT_BROWSERS_PATH resolves Chromium
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
			return fmt.Errorf("login failed: %w\n%s", err, out.String())
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
