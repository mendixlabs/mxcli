// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"strings"
	"testing"
)

func TestScreenshotArgs_TimeoutDefault(t *testing.T) {
	args := screenshotArgs(ScreenshotOptions{URL: "http://x:8080/", OutPath: "/o.png"})
	joined := strings.Join(args, " ")
	if !strings.HasPrefix(joined, "screenshot ") {
		t.Errorf("args should start with the screenshot subcommand: %v", args)
	}
	if !strings.Contains(joined, "--wait-for-timeout 4000") {
		t.Errorf("expected default 4000ms wait: %v", args)
	}
	// URL and out path are the trailing positional args, in order.
	if args[len(args)-2] != "http://x:8080/" || args[len(args)-1] != "/o.png" {
		t.Errorf("positional args wrong: %v", args)
	}
}

func TestScreenshotArgs_SelectorWins(t *testing.T) {
	args := screenshotArgs(ScreenshotOptions{URL: "u", OutPath: "o", Selector: ".mx-name-x", WaitMs: 9000})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--wait-for-selector .mx-name-x") {
		t.Errorf("expected selector wait: %v", args)
	}
	if strings.Contains(joined, "--wait-for-timeout") {
		t.Errorf("selector should replace the timeout wait: %v", args)
	}
}

func TestScreenshotArgs_FullPageAndViewport(t *testing.T) {
	args := screenshotArgs(ScreenshotOptions{URL: "u", OutPath: "o", FullPage: true, Viewport: "1280,800"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--full-page") {
		t.Errorf("expected --full-page: %v", args)
	}
	if !strings.Contains(joined, "--viewport-size 1280,800") {
		t.Errorf("expected viewport: %v", args)
	}
}

func TestCaptureScreenshot_NoOutPath(t *testing.T) {
	if err := CaptureScreenshot(ScreenshotOptions{URL: "u"}); err == nil {
		t.Error("expected error when OutPath is empty")
	}
}

func TestScreenshotArgs_LoadStorage(t *testing.T) {
	args := screenshotArgs(ScreenshotOptions{URL: "u", OutPath: "o", Storage: "/s.json"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--load-storage /s.json") {
		t.Errorf("expected --load-storage: %v", args)
	}
}
