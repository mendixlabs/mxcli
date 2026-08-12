// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/mendixlabs/mxcli/sdk/mpr"
)

const mxToolsetMaxPathUTF16Units = 259

type newProjectOutputState struct {
	path        string
	preexisting bool
	info        os.FileInfo
}

type newProjectCreation struct {
	mprPath        string
	version        string
	removedLocales int
}

// inspectNewProjectOutput rejects an existing non-empty output before the
// expensive MxBuild lookup and records the identity needed for safe rollback.
func inspectNewProjectOutput(outputDir string) (newProjectOutputState, error) {
	state := newProjectOutputState{path: outputDir}
	info, err := os.Lstat(outputDir)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("checking output directory %s: %w", outputDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		info, err = os.Stat(outputDir)
		if err != nil {
			return state, fmt.Errorf("resolving output directory %s: %w", outputDir, err)
		}
	}
	if !info.IsDir() {
		return state, fmt.Errorf("output path %s exists and is not a directory", outputDir)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return state, fmt.Errorf("reading output directory %s: %w", outputDir, err)
	}
	if len(entries) != 0 {
		return state, fmt.Errorf("directory %s already exists and is not empty", outputDir)
	}
	state.preexisting = true
	state.info = info
	return state, nil
}

// validateNewProjectOutputPath inspects the blank-project ZIP embedded in the
// resolved MxToolset version. MxToolset rejects a write at 260 UTF-16 code
// units on every OS, so reject the output before extraction can leave debris.
func validateNewProjectOutputPath(outputDir, appName, mendixVersion, mxPath string) error {
	longestRelativePath, err := longestBlankProjectTemplatePath(mxPath)
	if err != nil {
		return fmt.Errorf("inspecting the Mendix %s blank project template: %w", mendixVersion, err)
	}
	if mprName := appName + ".mpr"; utf16PathLen(mprName) > utf16PathLen(longestRelativePath) {
		longestRelativePath = mprName
	}
	return validateNewProjectPathLength(outputDir, mendixVersion, longestRelativePath)
}

func validateNewProjectPathLength(outputDir, mendixVersion, longestRelativePath string) error {
	projectedPath := filepath.Join(mxToolsetOutputPath(outputDir), filepath.FromSlash(longestRelativePath))
	projectedLength := utf16PathLen(projectedPath)
	if projectedLength <= mxToolsetMaxPathUTF16Units {
		return nil
	}

	maxOutputLength := mxToolsetMaxPathUTF16Units - utf16PathLen(filepath.FromSlash(longestRelativePath)) - 1
	return fmt.Errorf(
		"output directory is too long for Mendix %s project creation: %s\n"+
			"  The longest generated path would be %d UTF-16 code units; MxToolset supports at most %d.\n"+
			"  Use a shorter --output-dir (at most %d UTF-16 code units for this version)",
		mendixVersion, outputDir, projectedLength, mxToolsetMaxPathUTF16Units, maxOutputLength,
	)
}

// Windows retains a junction/subst spelling as the current directory, which is
// a useful explicit short-path workaround. Unix getcwd resolves symlinks, so
// account for the physical prefix there to avoid underestimating MxToolset's
// path string when the output itself does not exist yet.
func mxToolsetOutputPath(outputDir string) string {
	if runtime.GOOS == "windows" {
		return outputDir
	}

	current := outputDir
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved
		}
		if !os.IsNotExist(err) {
			return outputDir
		}
		parent := filepath.Dir(current)
		if parent == current {
			return outputDir
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func utf16PathLen(value string) int {
	return len(utf16.Encode([]rune(value)))
}

// longestBlankProjectTemplatePath locates the largest embedded ZIP with a root
// .mpr in Mendix.Modeler.Core.dll. This selects the NewProject resource over the
// much smaller SystemProject resource without depending on .resources internals.
func longestBlankProjectTemplatePath(mxPath string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(mxPath); err == nil {
		mxPath = resolved
	}
	corePath := filepath.Join(filepath.Dir(mxPath), "Mendix.Modeler.Core.dll")
	data, err := os.ReadFile(corePath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", corePath, err)
	}

	var bestPath string
	bestEntryCount := -1
	for searchEnd := len(data); searchEnd >= 4; {
		eocd := bytes.LastIndex(data[:searchEnd], []byte{'P', 'K', 5, 6})
		if eocd < 0 {
			break
		}
		searchEnd = eocd
		if eocd+22 > len(data) {
			continue
		}

		commentLength := int(binary.LittleEndian.Uint16(data[eocd+20 : eocd+22]))
		archiveEnd := eocd + 22 + commentLength
		centralSize := int(binary.LittleEndian.Uint32(data[eocd+12 : eocd+16]))
		centralOffset := int(binary.LittleEndian.Uint32(data[eocd+16 : eocd+20]))
		archiveStart := eocd - centralSize - centralOffset
		if archiveStart < 0 || archiveEnd > len(data) || archiveStart >= archiveEnd {
			continue
		}

		archiveData := data[archiveStart:archiveEnd]
		reader, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
		if err != nil {
			continue
		}

		hasRootMPR := false
		longest := ""
		for _, file := range reader.File {
			name := strings.TrimPrefix(file.Name, "./")
			if !strings.Contains(name, "/") && strings.EqualFold(path.Ext(name), ".mpr") {
				hasRootMPR = true
			}
			if !file.FileInfo().IsDir() && utf16PathLen(name) > utf16PathLen(longest) {
				longest = name
			}
		}
		if hasRootMPR && longest != "" && len(reader.File) > bestEntryCount {
			bestPath = longest
			bestEntryCount = len(reader.File)
		}
	}

	if bestPath == "" {
		return "", fmt.Errorf("blank project archive not found in %s", corePath)
	}
	return bestPath, nil
}

// createMendixProjectWithRollback runs MxToolset only after the path preflight.
// Any extraction or validation failure removes output created by this command;
// a pre-existing empty output directory itself is preserved.
func createMendixProjectWithRollback(
	outputDir, appName, expectedVersion, mxPath string,
	create func(projectDir string) error,
) (result newProjectCreation, resultErr error) {
	state, err := inspectNewProjectOutput(outputDir)
	if err != nil {
		return result, err
	}

	createdDirs, err := ensureNewProjectOutputDirectory(outputDir)
	if err != nil {
		return result, err
	}
	if !state.preexisting {
		state.info, err = os.Stat(outputDir)
		if err != nil {
			for i := len(createdDirs) - 1; i >= 0; i-- {
				_ = os.Remove(createdDirs[i])
			}
			return result, err
		}
	}

	defer func() {
		if resultErr == nil {
			return
		}
		cleanupErr := rollbackNewProjectOutput(state, createdDirs)
		if cleanupErr != nil {
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()

	if err := create(outputDir); err != nil {
		return result, fmt.Errorf("creating project: %w", err)
	}

	result.removedLocales = cleanupDuplicateLocaleFiles(outputDir)
	mprPath, err := findCreatedMPR(outputDir, appName)
	if err != nil {
		return result, err
	}

	if reader, err := mpr.Open(mprPath); err == nil {
		result.version = reader.ProjectVersion().ProductVersion
		reader.Close()
		if result.version != "" && result.version != expectedVersion {
			return result, fmt.Errorf(
				"requested Mendix %s but the created project is %s\n"+
					"  mx create-project stamps the project with the version of the binary that ran it (%s)\n"+
					"  Run 'mxcli setup mxbuild --version %s' and try again",
				expectedVersion, result.version, mxPath, expectedVersion,
			)
		}
	}

	result.mprPath = mprPath
	return result, nil
}

func findCreatedMPR(projectDir, appName string) (string, error) {
	preferred := filepath.Join(projectDir, appName+".mpr")
	if _, err := os.Stat(preferred); err == nil {
		return preferred, nil
	}
	fallback := filepath.Join(projectDir, "App.mpr")
	if _, err := os.Stat(fallback); err == nil {
		return fallback, nil
	}
	matches, _ := filepath.Glob(filepath.Join(projectDir, "*.mpr"))
	if len(matches) != 0 {
		return matches[0], nil
	}
	return "", fmt.Errorf("mx create-project did not produce an .mpr file in %s", projectDir)
}

func ensureNewProjectOutputDirectory(outputDir string) ([]string, error) {
	var missing []string
	for current := outputDir; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("output parent %s is not a directory", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		missing = append(missing, current)
		next := filepath.Dir(current)
		if next == current {
			return nil, fmt.Errorf("no existing parent for output directory %s", outputDir)
		}
	}

	created := make([]string, 0, len(missing))
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0o755); err != nil {
			for j := len(created) - 1; j >= 0; j-- {
				_ = os.Remove(created[j])
			}
			return nil, err
		}
		created = append(created, missing[i])
	}
	return created, nil
}

func rollbackNewProjectOutput(state newProjectOutputState, createdDirs []string) error {
	info, err := os.Stat(state.path)
	if err != nil {
		if os.IsNotExist(err) && !state.preexisting {
			removeNewProjectParentDirs(createdDirs, true)
			return nil
		}
		return fmt.Errorf("rolling back project output %s: %w", state.path, err)
	}
	if !os.SameFile(state.info, info) {
		return fmt.Errorf("refusing to roll back project output %s because the directory was replaced", state.path)
	}

	if state.preexisting {
		entries, err := os.ReadDir(state.path)
		if err != nil {
			return fmt.Errorf("reading project output during rollback: %w", err)
		}
		var cleanupErrors []error
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(state.path, entry.Name())); err != nil {
				cleanupErrors = append(cleanupErrors, err)
			}
		}
		if err := errors.Join(cleanupErrors...); err != nil {
			return fmt.Errorf("cleaning failed project from %s: %w", state.path, err)
		}
		return nil
	}

	if err := os.RemoveAll(state.path); err != nil {
		return fmt.Errorf("removing failed project output %s: %w", state.path, err)
	}
	removeNewProjectParentDirs(createdDirs, true)
	return nil
}

func removeNewProjectParentDirs(createdDirs []string, outputRemoved bool) {
	start := len(createdDirs) - 1
	if outputRemoved {
		start--
	}
	for i := start; i >= 0; i-- {
		_ = os.Remove(createdDirs[i])
	}
}
