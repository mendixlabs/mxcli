// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// screenshot.go captures a screenshot of the running app with Playwright's
// built-in `screenshot` subcommand. This is the standard `playwright` CLI (from
// @playwright/test), not the `playwright-cli` session tool used by
// `mxcli playwright` — it needs no live session and resolves Chromium from
// PLAYWRIGHT_BROWSERS_PATH, so it works in the devcontainer out of the box.
//
// Paired with the warm loop it gives a pixel-perfect page cycle: edit -> serve
// rebuild -> incremental client bundle -> hot apply -> re-screenshot.

// ScreenshotOptions configures CaptureScreenshot.
type ScreenshotOptions struct {
	URL      string        // page to shoot (default the app root)
	OutPath  string        // output PNG path (required)
	Selector string        // wait for this selector before shooting (optional)
	WaitMs   int           // fixed wait before shooting, ms (default 4000)
	FullPage bool          // capture the full scrollable page
	Viewport string        // "W,H" (e.g. "1280,800")
	Storage  string        // Playwright storage-state JSON to load (for authed pages)
	Timeout  time.Duration // overall command timeout (default 90s)
}

// resolvePlaywright returns the command + leading args to invoke the Playwright
// CLI: the `playwright` binary if on PATH, else `npx playwright`. The bool is
// false when neither is available.
func resolvePlaywright() (string, []string, bool) {
	if p, err := exec.LookPath("playwright"); err == nil {
		return p, nil, true
	}
	if p, err := exec.LookPath("npx"); err == nil {
		return p, []string{"playwright"}, true
	}
	return "", nil, false
}

// screenshotArgs builds the `screenshot` subcommand arguments (excluding the CLI
// prefix). Kept separate so it is unit-testable without invoking Playwright.
func screenshotArgs(opts ScreenshotOptions) []string {
	args := []string{"screenshot"}
	if opts.Selector != "" {
		args = append(args, "--wait-for-selector", opts.Selector)
	} else {
		wait := opts.WaitMs
		if wait == 0 {
			wait = 4000
		}
		args = append(args, "--wait-for-timeout", strconv.Itoa(wait))
	}
	if opts.FullPage {
		args = append(args, "--full-page")
	}
	if opts.Viewport != "" {
		args = append(args, "--viewport-size", opts.Viewport)
	}
	if opts.Storage != "" {
		args = append(args, "--load-storage", opts.Storage)
	}
	return append(args, opts.URL, opts.OutPath)
}

// CaptureScreenshot shoots opts.URL to opts.OutPath. Returns a clear error when
// the Playwright CLI is unavailable so the caller can degrade gracefully.
func CaptureScreenshot(opts ScreenshotOptions) error {
	if opts.OutPath == "" {
		return fmt.Errorf("screenshot output path is required")
	}
	bin, prefix, ok := resolvePlaywright()
	if !ok {
		return fmt.Errorf("playwright CLI not found (install with: npm i -g playwright); " +
			"Chromium is resolved from PLAYWRIGHT_BROWSERS_PATH")
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}

	args := append(append([]string{}, prefix...), screenshotArgs(opts)...)
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ() // carries PLAYWRIGHT_BROWSERS_PATH
	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching playwright: %w", err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("playwright screenshot failed: %w\n%s", err, out.String())
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("playwright screenshot timed out after %s", timeout)
	}
	if _, err := os.Stat(opts.OutPath); err != nil {
		return fmt.Errorf("playwright reported success but %s is missing:\n%s", opts.OutPath, out.String())
	}
	return nil
}
