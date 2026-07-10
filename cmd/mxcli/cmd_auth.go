// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/internal/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// validateURL is the endpoint hit to check whether a stored PAT is valid.
// GET returns 200 on a working credential and 401/403 on rejection.
// Variable (not const) so tests can point it at a local httptest.Server.
var validateURL = "https://marketplace-api.mendix.com/v1/content?limit=1"

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Mendix platform credentials",
	Long: `Manage Personal Access Tokens for Mendix platform APIs.

Credentials are used by marketplace (module install), catalog, and other
platform API features. A Personal Access Token (PAT) is created at:

  https://user-settings.mendix.com/

Storage priority (highest first):
  1. MENDIX_PAT env var (set MXCLI_PROFILE to target a non-default profile)
  2. ~/.mxcli/auth.json (mode 0600)

Multiple named profiles are supported for users with multiple Mendix tenants
or separate personal and CI credentials. The default profile name is "default".`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Store a Personal Access Token",
	Long: `Store a Mendix Personal Access Token for use by other mxcli commands.

Create a PAT at https://user-settings.mendix.com/ (Developer Settings →
Personal Access Tokens) and paste it when prompted. The token is validated
against the Mendix marketplace API before being stored.

Examples:
  mxcli auth login                             # interactive
  mxcli auth login --token "$MENDIX_PAT"       # non-interactive, for CI
  mxcli auth login --profile ci --token ...    # named profile`,
	RunE: runAuthLogin,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove a stored credential",
	RunE:  runAuthLogout,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current credential's status",
	RunE:  runAuthStatus,
}

var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored profiles",
	RunE:  runAuthList,
}

func init() {
	authLoginCmd.Flags().String("profile", auth.ProfileDefault, "credential profile name")
	authLoginCmd.Flags().String("token", "", "PAT value (non-interactive; omit to be prompted)")
	authLoginCmd.Flags().Bool("no-validate", false, "skip validating the PAT against the marketplace API before storing")

	authLogoutCmd.Flags().String("profile", auth.ProfileDefault, "credential profile name")
	authLogoutCmd.Flags().Bool("all", false, "remove all stored profiles")

	authStatusCmd.Flags().String("profile", auth.ProfileDefault, "credential profile name")
	authStatusCmd.Flags().Bool("json", false, "emit machine-readable JSON")
	authStatusCmd.Flags().Bool("offline", false, "skip validation against the marketplace API")

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authListCmd)

	rootCmd.AddCommand(authCmd)
}

func runAuthLogin(cmd *cobra.Command, _ []string) error {
	profile, _ := cmd.Flags().GetString("profile")
	token, _ := cmd.Flags().GetString("token")
	noValidate, _ := cmd.Flags().GetBool("no-validate")

	if token == "" {
		var err error
		token, err = promptForToken(cmd.OutOrStdout())
		if err != nil {
			return err
		}
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("no token provided")
	}

	if !noValidate {
		fmt.Fprint(cmd.OutOrStdout(), "Validating... ")
		if err := validatePAT(cmd.Context(), token); err != nil {
			fmt.Fprintln(cmd.OutOrStdout(), "✗")
			return fmt.Errorf("PAT rejected by Mendix: %w\nhint: confirm the token at https://user-settings.mendix.com/", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "✓")
	}

	store, err := auth.DefaultFileStore()
	if err != nil {
		return err
	}
	cred := &auth.Credential{
		Scheme:    auth.SchemePAT,
		Token:     token,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Put(profile, cred); err != nil {
		return fmt.Errorf("save credential: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Saved credential to profile %q (scheme: %s)\n", profile, cred.Scheme)
	return nil
}

func runAuthLogout(cmd *cobra.Command, _ []string) error {
	profile, _ := cmd.Flags().GetString("profile")
	all, _ := cmd.Flags().GetBool("all")

	store, err := auth.DefaultFileStore()
	if err != nil {
		return err
	}

	if all {
		profiles, err := store.List()
		if err != nil {
			return err
		}
		for _, p := range profiles {
			if err := store.Delete(p); err != nil {
				return fmt.Errorf("delete profile %q: %w", p, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed profile %q\n", p)
		}
		if len(profiles) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No stored profiles.")
		}
		return nil
	}

	if err := store.Delete(profile); err != nil {
		return fmt.Errorf("delete profile %q: %w", profile, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed profile %q\n", profile)
	return nil
}

func runAuthStatus(cmd *cobra.Command, _ []string) error {
	profile, _ := cmd.Flags().GetString("profile")
	asJSON, _ := cmd.Flags().GetBool("json")
	offline, _ := cmd.Flags().GetBool("offline")

	cred, err := auth.Resolve(cmd.Context(), profile)
	if err != nil {
		return err
	}

	source := "file (~/.mxcli/auth.json)"
	if os.Getenv(auth.EnvPAT) != "" && envProfileMatches(profile) {
		source = "env (MENDIX_PAT)"
	}

	valid := ""
	if !offline {
		if err := validatePAT(cmd.Context(), cred.Token); err == nil {
			valid = "ok"
		} else {
			valid = fmt.Sprintf("rejected (%v)", err)
		}
	}

	if asJSON {
		out := map[string]any{
			"profile":    profile,
			"scheme":     string(cred.Scheme),
			"source":     source,
			"created_at": cred.CreatedAt,
		}
		if valid != "" {
			out["valid"] = valid == "ok"
			out["valid_detail"] = valid
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Profile:\t%s\n", profile)
	fmt.Fprintf(w, "Scheme:\t%s\n", cred.Scheme)
	fmt.Fprintf(w, "Source:\t%s\n", source)
	fmt.Fprintf(w, "Created:\t%s\n", cred.CreatedAt.Format(time.RFC3339))
	if valid != "" {
		fmt.Fprintf(w, "Validated:\t%s\n", valid)
	}
	return w.Flush()
}

func runAuthList(cmd *cobra.Command, _ []string) error {
	store, err := auth.DefaultFileStore()
	if err != nil {
		return err
	}
	profiles, err := store.List()
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No stored profiles. Run: mxcli auth login")
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROFILE\tSCHEME\tCREATED")
	for _, p := range profiles {
		cred, err := store.Get(p)
		if err != nil {
			fmt.Fprintf(w, "%s\t(error: %v)\t\n", p, err)
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", p, cred.Scheme, cred.CreatedAt.Format(time.RFC3339))
	}
	return w.Flush()
}

// promptForToken reads a PAT from stdin. Uses term.ReadPassword (no echo)
// when stdin is a TTY; falls back to echoed bufio.Scanner with a warning
// when stdin is piped (common in devcontainers and CI that still want
// interactive-ish usage).
func promptForToken(out io.Writer) (string, error) {
	fmt.Fprintln(out, "Create a PAT at: https://user-settings.mendix.com/")
	fmt.Fprintln(out, "  (Developer Settings → Personal Access Tokens)")
	fmt.Fprintln(out)
	fmt.Fprint(out, "PAT: ")

	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("read token: %w", err)
		}
		return string(b), nil
	}
	// Non-TTY: read a line from stdin. Warn that echo is on since we can't
	// disable it, so the user knows not to type the token interactively.
	fmt.Fprintln(out)
	fmt.Fprintln(out, "(stdin is not a terminal — token will be echoed; prefer --token for scripts)")
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no input on stdin")
	}
	return sc.Text(), nil
}

// validatePAT makes a single GET to the marketplace-api content endpoint
// with the given PAT. Returns nil on 200. Wraps 401/403 as
// *auth.ErrUnauthenticated. Network errors and other statuses are returned
// as plain errors.
func validatePAT(ctx context.Context, token string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", validateURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "MxToken "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("network: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return &auth.ErrUnauthenticated{}
	default:
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, validateURL)
	}
}

// envProfileMatches reports whether env vars are currently populating the
// given profile. Mirrors the resolver's internal logic.
func envProfileMatches(profile string) bool {
	env := strings.TrimSpace(os.Getenv(auth.EnvProfile))
	if env == "" {
		env = auth.ProfileDefault
	}
	return env == profile
}

// Compile-time interface assertion so we notice if auth.ErrUnauthenticated
// stops satisfying error.
var _ error = (*auth.ErrUnauthenticated)(nil)
