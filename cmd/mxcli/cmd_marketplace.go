// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/JordtenBulte-OLC/mxcli/internal/auth"
	"github.com/JordtenBulte-OLC/mxcli/internal/marketplace"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var marketplaceCmd = &cobra.Command{
	Use:   "marketplace",
	Short: "Browse the Mendix marketplace",
	Long: `Browse and download published modules, widgets, and themes in the Mendix marketplace.

Requires a Personal Access Token (PAT). Run 'mxcli auth login' first.

'marketplace download' fetches a content version's .mpk to disk. To install a
downloaded module into a project, use Studio Pro or 'mx module-import'.`,
}

var marketplaceSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search marketplace content by keyword",
	Long: `Search marketplace content by keyword (matches item name and publisher).

The Content API has no server-side search, so the first search fetches the full
catalog listing and caches it under ~/.mxcli/ for a day. Subsequent searches
(any query) are served from that cache instantly. Use --refresh to re-fetch.`,
	Example: `  mxcli marketplace search "database connector"
  mxcli marketplace search "audit" --limit 5 --json
  mxcli marketplace search "external database" --refresh`,
	Args: cobra.ExactArgs(1),
	RunE: runMarketplaceSearch,
}

var marketplaceInfoCmd = &cobra.Command{
	Use:   "info <content-id>",
	Short: "Show details of a marketplace item",
	Example: `  mxcli marketplace info 170
  mxcli marketplace info 2888 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runMarketplaceInfo,
}

var marketplaceVersionsCmd = &cobra.Command{
	Use:   "versions <content-id>",
	Short: "List available versions of a marketplace item",
	Example: `  mxcli marketplace versions 2888
  mxcli marketplace versions 170 --min-mendix 10.24.0
  mxcli marketplace versions 170 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runMarketplaceVersions,
}

var marketplaceDownloadCmd = &cobra.Command{
	Use:   "download <content-id>",
	Short: "Download a marketplace item's .mpk to disk",
	Long: `Download the .mpk package for a marketplace content version.

By default the latest version is downloaded. Use --version to pick a specific
version number. The file is written to the current directory under its CDN
filename unless -o is given.`,
	Example: `  mxcli marketplace download 2888
  mxcli marketplace download 2888 --version 7.0.2
  mxcli marketplace download 2888 -o ./mods/dbc.mpk`,
	Args: cobra.ExactArgs(1),
	RunE: runMarketplaceDownload,
}

func init() {
	marketplaceSearchCmd.Flags().IntP("limit", "n", 20, "max results")
	marketplaceSearchCmd.Flags().String("profile", auth.ProfileDefault, "credential profile")
	marketplaceSearchCmd.Flags().Bool("json", false, "emit JSON instead of a table")
	marketplaceSearchCmd.Flags().Bool("refresh", false, "bypass the catalog cache and fetch fresh from the marketplace")

	marketplaceInfoCmd.Flags().String("profile", auth.ProfileDefault, "credential profile")
	marketplaceInfoCmd.Flags().Bool("json", false, "emit JSON instead of a table")

	marketplaceVersionsCmd.Flags().String("profile", auth.ProfileDefault, "credential profile")
	marketplaceVersionsCmd.Flags().Bool("json", false, "emit JSON instead of a table")
	marketplaceVersionsCmd.Flags().String("min-mendix", "", "filter versions whose minSupportedMendixVersion is <= this (e.g., 10.24.0)")

	marketplaceDownloadCmd.Flags().String("profile", auth.ProfileDefault, "credential profile")
	marketplaceDownloadCmd.Flags().String("version", "", "version number to download (default: latest)")
	marketplaceDownloadCmd.Flags().StringP("output", "o", "", "output path (default: CDN filename in the current directory)")

	marketplaceCmd.AddCommand(marketplaceSearchCmd)
	marketplaceCmd.AddCommand(marketplaceInfoCmd)
	marketplaceCmd.AddCommand(marketplaceVersionsCmd)
	marketplaceCmd.AddCommand(marketplaceDownloadCmd)

	rootCmd.AddCommand(marketplaceCmd)
}

// resolveVersion picks a version from a content item's version list: the one
// matching versionNumber, or the latest (first, per the API ordering) when
// versionNumber is empty.
func resolveVersion(versions []marketplace.Version, versionNumber string) (*marketplace.Version, error) {
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions available for this content")
	}
	if versionNumber == "" {
		return &versions[0], nil
	}
	for i := range versions {
		if versions[i].VersionNumber == versionNumber {
			return &versions[i], nil
		}
	}
	return nil, fmt.Errorf("version %q not found; run 'mxcli marketplace versions <id>' to list available versions", versionNumber)
}

func runMarketplaceDownload(cmd *cobra.Command, args []string) error {
	contentID, err := parseContentID(args[0])
	if err != nil {
		return err
	}
	versionNumber, _ := cmd.Flags().GetString("version")
	output, _ := cmd.Flags().GetString("output")

	client, err := newMarketplaceClient(cmd.Context(), cmd)
	if err != nil {
		return err
	}

	list, err := client.Versions(cmd.Context(), contentID)
	if err != nil {
		return err
	}
	version, err := resolveVersion(list.Items, versionNumber)
	if err != nil {
		return err
	}
	if version.DownloadURL == "" {
		return fmt.Errorf("version %s exposes no download URL", version.VersionNumber)
	}

	// Stream to a temp file first, then derive the final name and rename, so a
	// failed/cancelled download never leaves a truncated .mpk in place.
	dir := "."
	if output != "" {
		dir = filepath.Dir(output)
	}
	tmp, err := os.CreateTemp(dir, ".mxcli-download-*.part")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	filename, derr := client.Download(cmd.Context(), version, tmp)
	closeErr := tmp.Close()
	if derr != nil {
		return derr
	}
	if closeErr != nil {
		return closeErr
	}

	dest := output
	if dest == "" {
		if filename == "" {
			filename = fmt.Sprintf("content-%d-%s.mpk", contentID, version.VersionNumber)
		}
		dest = filename
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}

	info, _ := os.Stat(dest)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Downloaded %s (%s, %.1f MB)\n", dest, version.VersionNumber, float64(size)/(1024*1024))
	return nil
}

// newMarketplaceClient builds an authenticated marketplace client using
// the profile flag on the given command. Overrideable by tests via
// marketplaceClientFactory.
func newMarketplaceClient(ctx context.Context, cmd *cobra.Command) (*marketplace.Client, error) {
	if marketplaceClientFactory != nil {
		return marketplaceClientFactory(ctx, cmd)
	}
	profile, _ := cmd.Flags().GetString("profile")
	httpClient, err := auth.ClientFor(ctx, profile)
	if err != nil {
		return nil, fmt.Errorf("%w\nhint: run 'mxcli auth login'", err)
	}
	return marketplace.New(httpClient), nil
}

// marketplaceClientFactory, if set, overrides the default auth-backed
// client construction. Tests use this to inject an httptest-backed client.
var marketplaceClientFactory func(context.Context, *cobra.Command) (*marketplace.Client, error)

func runMarketplaceSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	limit, _ := cmd.Flags().GetInt("limit")
	asJSON, _ := cmd.Flags().GetBool("json")
	refresh, _ := cmd.Flags().GetBool("refresh")
	profile, _ := cmd.Flags().GetString("profile")

	client, err := newMarketplaceClient(cmd.Context(), cmd)
	if err != nil {
		return err
	}

	// The Content API has no server-side search, so a rare query scans much of
	// the catalog. Show progress on an interactive terminal so it doesn't look
	// hung. Skipped for --json and non-terminal stderr (pipes, CI).
	stderr := cmd.ErrOrStderr()
	if progressDone := startSearchProgress(client, stderr, asJSON); progressDone != nil {
		defer progressDone()
	}

	// Serve from the on-disk catalog cache when fresh so repeat searches are
	// instant; otherwise fetch the full catalog once, cache it, and filter.
	cache := &marketplace.CatalogCache{Path: marketplaceCachePath(profile)}
	list, fromCache, err := marketplace.SearchCached(cmd.Context(), client, cache, query, limit, refresh)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(cmd, list)
	}
	if fromCache {
		fmt.Fprintln(stderr, "(results from cached catalog; pass --refresh to update)")
	}
	return renderContentTable(cmd, list.Items)
}

// marketplaceCachePath returns the catalog cache file for a credential profile
// (~/.mxcli/marketplace-catalog-<profile>.json). Keyed by profile so different
// accounts don't share a cache.
func marketplaceCachePath(profile string) string {
	if profile == "" {
		profile = auth.ProfileDefault
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".mxcli", "marketplace-catalog-"+profile+".json")
}

// startSearchProgress wires a live "scanning" indicator on client.OnProgress
// when stderr is an interactive terminal and output isn't JSON. It returns a
// cleanup func that clears the progress line, or nil when no progress is shown.
func startSearchProgress(client *marketplace.Client, stderr io.Writer, asJSON bool) func() {
	if asJSON {
		return nil
	}
	f, ok := stderr.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return nil
	}
	client.OnProgress = func(scanned int) {
		fmt.Fprintf(stderr, "\rSearching marketplace… %d items scanned", scanned)
	}
	return func() {
		fmt.Fprint(stderr, "\r\033[K") // clear the progress line
	}
}

func runMarketplaceInfo(cmd *cobra.Command, args []string) error {
	contentID, err := parseContentID(args[0])
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")

	client, err := newMarketplaceClient(cmd.Context(), cmd)
	if err != nil {
		return err
	}
	content, err := client.Get(cmd.Context(), contentID)
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(cmd, content)
	}
	return renderContentDetail(cmd, content)
}

func runMarketplaceVersions(cmd *cobra.Command, args []string) error {
	contentID, err := parseContentID(args[0])
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	minMendix, _ := cmd.Flags().GetString("min-mendix")

	client, err := newMarketplaceClient(cmd.Context(), cmd)
	if err != nil {
		return err
	}
	list, err := client.Versions(cmd.Context(), contentID)
	if err != nil {
		return err
	}

	items := list.Items
	if minMendix != "" {
		items = filterVersionsByMinMendix(items, minMendix)
	}

	if asJSON {
		return emitJSON(cmd, &marketplace.VersionList{Items: items})
	}
	return renderVersionsTable(cmd, items)
}

// filterVersionsByMinMendix returns only versions whose
// minSupportedMendixVersion is <= the provided version. Used to narrow
// results to versions compatible with a target project.
func filterVersionsByMinMendix(versions []marketplace.Version, maxVer string) []marketplace.Version {
	out := make([]marketplace.Version, 0, len(versions))
	for _, v := range versions {
		if compareSemverLike(v.MinSupportedMendixVersion, maxVer) <= 0 {
			out = append(out, v)
		}
	}
	return out
}

// compareSemverLike compares two dotted version strings numerically.
// Returns -1, 0, or 1. Missing components are treated as 0. Non-numeric
// components fall back to string comparison for that component.
func compareSemverLike(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := max(len(as), len(bs))
	for i := range n {
		aa, bb := "0", "0"
		if i < len(as) {
			aa = as[i]
		}
		if i < len(bs) {
			bb = bs[i]
		}
		ai, aerr := atoiSafe(aa)
		bi, berr := atoiSafe(bb)
		if aerr == nil && berr == nil {
			if ai < bi {
				return -1
			}
			if ai > bi {
				return 1
			}
			continue
		}
		if aa < bb {
			return -1
		}
		if aa > bb {
			return 1
		}
	}
	return 0
}

func atoiSafe(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("non-numeric: %q", s)
	}
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("non-numeric: %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func parseContentID(s string) (int, error) {
	n, err := atoiSafe(s)
	if err != nil {
		return 0, fmt.Errorf("invalid content id %q: must be a positive integer", s)
	}
	return n, nil
}

// emitJSON pretty-prints v as JSON to cmd.OutOrStdout.
func emitJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func renderContentTable(cmd *cobra.Command, items []marketplace.Content) error {
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No results.")
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tPUBLISHER\tSUPPORT\tLATEST\tNAME")
	for _, it := range items {
		latest := ""
		name := ""
		if it.LatestVersion != nil {
			latest = it.LatestVersion.VersionNumber
			name = it.LatestVersion.Name
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
			it.ContentID, it.Type, it.Publisher, it.SupportCategory, latest, name)
	}
	return w.Flush()
}

func renderContentDetail(cmd *cobra.Command, c *marketplace.Content) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Content ID:\t%d\n", c.ContentID)
	fmt.Fprintf(w, "Type:\t%s\n", c.Type)
	fmt.Fprintf(w, "Publisher:\t%s\n", c.Publisher)
	fmt.Fprintf(w, "Support:\t%s\n", c.SupportCategory)
	if len(c.Categories) > 0 {
		names := make([]string, 0, len(c.Categories))
		for _, cat := range c.Categories {
			names = append(names, cat.Name)
		}
		fmt.Fprintf(w, "Categories:\t%s\n", strings.Join(names, ", "))
	}
	if c.LicenseURL != "" {
		fmt.Fprintf(w, "License:\t%s\n", c.LicenseURL)
	}
	fmt.Fprintf(w, "Private:\t%v\n", c.IsPrivate)
	if c.LatestVersion != nil {
		v := c.LatestVersion
		fmt.Fprintf(w, "Latest:\t%s (%s)\n", v.VersionNumber, v.Name)
		fmt.Fprintf(w, "Min Mendix:\t%s\n", v.MinSupportedMendixVersion)
		fmt.Fprintf(w, "Published:\t%s\n", v.PublicationDate.Format("2006-01-02"))
	}
	return w.Flush()
}

func renderVersionsTable(cmd *cobra.Command, items []marketplace.Version) error {
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No versions.")
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tMIN MENDIX\tPUBLISHED\tNAME")
	for _, v := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			v.VersionNumber, v.MinSupportedMendixVersion,
			v.PublicationDate.Format("2006-01-02"), v.Name)
	}
	return w.Flush()
}
