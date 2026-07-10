// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/mdl/diaglog"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"github.com/spf13/cobra"
)

var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: "Show diagnostics and manage session logs",
	Long: `Show diagnostic information and manage session log files.

Examples:
  mxcli diag              # Show version, platform, log dir, recent errors
  mxcli diag --log-path   # Print log directory path
  mxcli diag --tail 20    # Show last 20 log entries
  mxcli diag --bundle     # Create tar.gz with logs for bug reports
`,
	Run: func(cmd *cobra.Command, args []string) {
		logPath, _ := cmd.Flags().GetBool("log-path")
		bundle, _ := cmd.Flags().GetBool("bundle")
		tail, _ := cmd.Flags().GetInt("tail")

		logDir := diaglog.LogDir()

		if logPath {
			fmt.Println(logDir)
			return
		}

		if bundle {
			runDiagBundle(logDir)
			return
		}

		checkUnits, _ := cmd.Flags().GetBool("check-units")
		fix, _ := cmd.Flags().GetBool("fix")
		if checkUnits {
			projectPath, _ := cmd.Flags().GetString("project")
			if projectPath == "" {
				fmt.Fprintln(os.Stderr, "Error: --check-units requires -p <project.mpr>")
				os.Exit(1)
			}
			runCheckUnits(projectPath, fix)
			return
		}

		if tail > 0 {
			runDiagTail(logDir, tail)
			return
		}

		runDiagInfo(logDir)
	},
}

func init() {
	diagCmd.Flags().Bool("log-path", false, "Print log directory path")
	diagCmd.Flags().Bool("bundle", false, "Create tar.gz with logs for bug reports")
	diagCmd.Flags().Int("tail", 0, "Show last N log entries")
	diagCmd.Flags().Bool("check-units", false, "Check for orphan units and stale mxunit files (MPR v2)")
	diagCmd.Flags().Bool("fix", false, "Auto-fix issues found by --check-units")
}

// runDiagInfo shows diagnostic summary.
func runDiagInfo(logDir string) {
	fmt.Println("mxcli diagnostics")
	fmt.Printf("  Version:     %s\n", version)
	fmt.Printf("  Go:          %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  Log dir:     %s\n", logDir)

	// Count log files and total size
	files, totalSize := listLogFiles(logDir)
	fmt.Printf("  Log files:   %d files (%s)\n", len(files), formatBytes(totalSize))

	// Count sessions and errors from recent logs
	sessions, errors := countSessionsAndErrors(logDir, files)
	fmt.Printf("  Sessions:    %d (last 7 days)\n", sessions)
	fmt.Printf("  Errors:      %d (last 7 days)\n", errors)

	// Show recent errors
	recentErrors := collectRecentErrors(logDir, files, 5)
	if len(recentErrors) > 0 {
		fmt.Println()
		fmt.Println("Recent errors:")
		for _, e := range recentErrors {
			fmt.Printf("  %s\n", e)
		}
	}
}

// runDiagTail shows the last N log entries.
func runDiagTail(logDir string, n int) {
	files, _ := listLogFiles(logDir)
	if len(files) == 0 {
		fmt.Println("No log files found.")
		return
	}

	// Collect lines from most recent files
	var allLines []string
	for i := len(files) - 1; i >= 0 && len(allLines) < n; i-- {
		lines := readFileLines(filepath.Join(logDir, files[i].Name()))
		allLines = append(lines, allLines...)
	}

	// Show last N
	start := 0
	if len(allLines) > n {
		start = len(allLines) - n
	}
	for _, line := range allLines[start:] {
		fmt.Println(line)
	}
}

// runDiagBundle creates a tar.gz archive of logs.
func runDiagBundle(logDir string) {
	timestamp := time.Now().Format("20060102-150405")
	outFile := fmt.Sprintf("mxcli-diag-%s.tar.gz", timestamp)

	f, err := os.Create(outFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating bundle: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add system info
	info := fmt.Sprintf("Version: %s\nGo: %s %s/%s\nTime: %s\n",
		version, runtime.Version(), runtime.GOOS, runtime.GOARCH, time.Now().Format(time.RFC3339))
	addTarEntry(tw, "system-info.txt", []byte(info))

	// Add all log files
	files, _ := listLogFiles(logDir)
	for _, entry := range files {
		path := filepath.Join(logDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		addTarEntry(tw, filepath.Join("logs", entry.Name()), data)
	}

	fmt.Printf("Created: %s\n", outFile)
}

// listLogFiles returns log file entries and total size, sorted by name (oldest first).
func listLogFiles(logDir string) ([]os.DirEntry, int64) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, 0
	}
	var files []os.DirEntry
	var totalSize int64
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "mxcli-") && strings.HasSuffix(e.Name(), ".log") {
			files = append(files, e)
			if info, err := e.Info(); err == nil {
				totalSize += info.Size()
			}
		}
	}
	return files, totalSize
}

// countSessionsAndErrors scans log files for session_start and error entries.
func countSessionsAndErrors(logDir string, files []os.DirEntry) (sessions, errors int) {
	for _, entry := range files {
		lines := readFileLines(filepath.Join(logDir, entry.Name()))
		for _, line := range lines {
			if strings.Contains(line, `"session_start"`) {
				sessions++
			}
			if strings.Contains(line, `"ERROR"`) {
				errors++
			}
		}
	}
	return
}

// collectRecentErrors extracts recent error summaries from log files.
func collectRecentErrors(logDir string, files []os.DirEntry, maxErrors int) []string {
	var errors []string
	// Read from newest to oldest
	for i := len(files) - 1; i >= 0 && len(errors) < maxErrors; i-- {
		lines := readFileLines(filepath.Join(logDir, files[i].Name()))
		for j := len(lines) - 1; j >= 0 && len(errors) < maxErrors; j-- {
			line := lines[j]
			if !strings.Contains(line, `"ERROR"`) {
				continue
			}
			var entry map[string]any
			if json.Unmarshal([]byte(line), &entry) != nil {
				continue
			}
			t, _ := entry["time"].(string)
			summary, _ := entry["stmt_summary"].(string)
			errMsg, _ := entry["error"].(string)
			if t != "" && errMsg != "" {
				// Parse and reformat time
				if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
					t = ts.Format("2006-01-02 15:04")
				}
				if summary != "" {
					errors = append(errors, fmt.Sprintf("[%s] %s: %s", t, summary, errMsg))
				} else {
					errors = append(errors, fmt.Sprintf("[%s] %s", t, errMsg))
				}
			}
		}
	}
	return errors
}

func readFileLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func addTarEntry(tw *tar.Writer, name string, data []byte) {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if tw.WriteHeader(hdr) == nil {
		io.Copy(tw, strings.NewReader(string(data)))
	}
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%d KB", b/1024)
}

// runCheckUnits checks for orphan units (Unit table entry without mxunit file)
// and stale mxunit files (file exists but no Unit table entry). MPR v2 only.
func runCheckUnits(mprPath string, fix bool) {
	reader, err := mpr.Open(mprPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	contentsDir := reader.ContentsDir()
	if contentsDir == "" {
		fmt.Println("Not an MPR v2 project (no mprcontents directory)")
		return
	}

	// Build set of unit UUIDs from database
	unitIDs, err := reader.ListAllUnitIDs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing units: %v\n", err)
		os.Exit(1)
	}
	unitSet := make(map[string]bool, len(unitIDs))
	for _, id := range unitIDs {
		unitSet[id] = true
	}

	// Scan mxunit files
	files, err := filepath.Glob(filepath.Join(contentsDir, "*", "*", "*.mxunit"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning mxunit files: %v\n", err)
		os.Exit(1)
	}
	fileSet := make(map[string]string, len(files)) // uuid → filepath
	for _, f := range files {
		uuid := strings.TrimSuffix(filepath.Base(f), ".mxunit")
		fileSet[uuid] = f
	}

	// Check for orphan units (in DB but no file)
	orphans := 0
	for _, id := range unitIDs {
		if _, ok := fileSet[id]; !ok {
			fmt.Printf("ORPHAN UNIT: %s (in Unit table but no mxunit file)\n", id)
			orphans++
		}
	}

	// Check for stale files (file exists but not in DB)
	stale := 0
	for uuid, fpath := range fileSet {
		if !unitSet[uuid] {
			fmt.Printf("STALE FILE:  %s\n", uuid)
			stale++
			if fix {
				if err := os.Remove(fpath); err != nil {
					fmt.Fprintf(os.Stderr, "  ERROR removing: %v\n", err)
				} else {
					fmt.Printf("  REMOVED:   %s\n", fpath)
					// Clean empty parent dirs
					dir2 := filepath.Dir(fpath)
					os.Remove(dir2)
					dir1 := filepath.Dir(dir2)
					os.Remove(dir1)
				}
			}
		}
	}

	fmt.Printf("\nSummary: %d units in DB, %d mxunit files, %d orphans, %d stale\n",
		len(unitIDs), len(files), orphans, stale)
	if stale > 0 && !fix {
		fmt.Println("Run with --fix to auto-remove stale files")
	}
}
