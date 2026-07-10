// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/internal/auth"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// withTestHome redirects HOME and MENDIX_* env vars so each test gets an
// isolated credential store without touching the user's ~/.mxcli.
func withTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(auth.EnvPAT, "")
	t.Setenv(auth.EnvProfile, "")
	return home
}

// runAuth executes the auth subtree with the given args and returns the
// combined output. Flags are reset to their defaults before each run so
// that values don't leak between tests running in the same process.
//
// Uses rootCmd.SetArgs + rootCmd.Execute because subcommand writers are
// inherited by walking up to the root; setting Out on authCmd alone does
// not propagate to authLoginCmd reliably.
func runAuth(t *testing.T, args ...string) (string, error) {
	t.Helper()
	for _, c := range []*cobra.Command{authLoginCmd, authLogoutCmd, authStatusCmd, authListCmd} {
		resetCmdFlags(c)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{"auth"}, args...))
	err := rootCmd.ExecuteContext(context.Background())
	return out.String(), err
}

// resetCmdFlags restores each flag on cmd to its default value. Needed
// because cobra/pflag retain values between Execute calls in the same
// process.
func resetCmdFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

// ---- tests ----

func TestAuthLogin_StoresCredential(t *testing.T) {
	home := withTestHome(t)

	out, err := runAuth(t, "login", "--token", "test-pat-123", "--no-validate")
	if err != nil {
		t.Fatalf("login: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Saved credential to profile") {
		t.Errorf("output missing confirmation: %s", out)
	}

	authPath := filepath.Join(home, ".mxcli", "auth.json")
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected mode 0600, got %o", mode)
	}
}

func TestAuthLogin_ValidationFailure(t *testing.T) {
	withTestHome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	orig := validateURL
	validateURL = ts.URL
	defer func() { validateURL = orig }()

	_, err := runAuth(t, "login", "--token", "bad-pat")
	if err == nil {
		t.Fatal("expected validation failure, got nil")
	}
	if !strings.Contains(err.Error(), "rejected") {
		t.Errorf("expected 'rejected' in error, got %v", err)
	}

	// No credential should be stored on validation failure.
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(home, ".mxcli", "auth.json")); !os.IsNotExist(err) {
		t.Errorf("credential file should not exist after validation failure, got err=%v", err)
	}
}

func TestAuthList_EmptyStore(t *testing.T) {
	withTestHome(t)
	out, err := runAuth(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "No stored profiles") {
		t.Errorf("expected empty-store message, got: %s", out)
	}
}

func TestAuthList_ShowsProfiles(t *testing.T) {
	withTestHome(t)
	if _, err := runAuth(t, "login", "--token", "t1", "--no-validate"); err != nil {
		t.Fatal(err)
	}
	if _, err := runAuth(t, "login", "--profile", "ci", "--token", "t2", "--no-validate"); err != nil {
		t.Fatal(err)
	}
	out, err := runAuth(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "default") || !strings.Contains(out, "ci") {
		t.Errorf("list should include both profiles, got: %s", out)
	}
}

func TestAuthStatus_JSONOutput(t *testing.T) {
	withTestHome(t)
	if _, err := runAuth(t, "login", "--token", "t1", "--no-validate"); err != nil {
		t.Fatal(err)
	}
	out, err := runAuth(t, "status", "--offline", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if got["profile"] != "default" {
		t.Errorf("profile field wrong: %v", got["profile"])
	}
	if got["scheme"] != "pat" {
		t.Errorf("scheme field wrong: %v", got["scheme"])
	}
	if _, ok := got["valid"]; ok {
		t.Errorf("--offline should not emit 'valid', got: %v", got)
	}
}

func TestAuthLogout_RemovesProfile(t *testing.T) {
	withTestHome(t)
	if _, err := runAuth(t, "login", "--token", "t1", "--no-validate"); err != nil {
		t.Fatal(err)
	}
	if _, err := runAuth(t, "logout"); err != nil {
		t.Fatal(err)
	}
	// Verify via the store, not a second runAuth call (cobra flag state
	// between back-to-back Execute calls is flaky enough that we'd rather
	// check persistent state directly).
	store, _ := auth.DefaultFileStore()
	var noCred *auth.ErrNoCredential
	if _, err := store.Get(auth.ProfileDefault); !errors.As(err, &noCred) {
		t.Errorf("expected ErrNoCredential after logout, got %v", err)
	}
}

func TestAuthLogout_All(t *testing.T) {
	withTestHome(t)
	_, _ = runAuth(t, "login", "--token", "t1", "--no-validate")
	_, _ = runAuth(t, "login", "--profile", "ci", "--token", "t2", "--no-validate")

	out, err := runAuth(t, "logout", "--all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "default") || !strings.Contains(out, "ci") {
		t.Errorf("--all should remove both profiles, got: %s", out)
	}

	store, _ := auth.DefaultFileStore()
	profiles, _ := store.List()
	if len(profiles) != 0 {
		t.Errorf("--all should remove every profile, still have: %v", profiles)
	}
}

func TestValidatePAT_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "MxToken good-pat" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	orig := validateURL
	validateURL = ts.URL
	defer func() { validateURL = orig }()

	if err := validatePAT(context.Background(), "good-pat"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidatePAT_401And403AreUnauthenticated(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))

		orig := validateURL
		validateURL = ts.URL

		err := validatePAT(context.Background(), "x")
		var unauth *auth.ErrUnauthenticated
		if !errors.As(err, &unauth) {
			t.Errorf("status %d: expected ErrUnauthenticated, got %v", status, err)
		}

		validateURL = orig
		ts.Close()
	}
}
