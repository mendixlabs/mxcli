// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr/version"
)

// PatchResult describes the outcome of applying a single patch.
type PatchResult struct {
	Description string
	Status      string // "applied", "skipped", "error"
	Error       error
}

// ApplyPatches applies version-aware patches to the PAD output directory.
// Returns results for each patch attempted.
func ApplyPatches(padDir string, pv *version.ProjectVersion) []PatchResult {
	var results []PatchResult

	is116x := pv.MajorVersion == 11 && pv.MinorVersion == 6

	// Patch 1: chmod start script (always — ZIP extraction doesn't preserve +x)
	results = append(results, patchStartPermissions(padDir))

	// Patch 2: CMD fix in Dockerfile — start.sh -> start (11.6.x only)
	if is116x {
		results = append(results, patchDockerfileCMD(padDir))
	}

	// Patch 3: Remove config arg from CMD (all versions)
	// bin/start accumulates args with CONFIG_FILES="$CONFIG_FILES $1" which
	// produces a leading space (" etc/Default") that the runtime rejects.
	// Let bin/start use its own DEFAULT_CONF variable instead.
	results = append(results, patchDockerfileCMDConfigArg(padDir))

	// Patch 4: Replace deprecated openjdk base image (all versions)
	results = append(results, patchBaseImage(padDir))

	// Patch 5: Add healthcheck (all versions)
	results = append(results, patchHealthcheck(padDir))

	// Patch 6: Bind admin API to all interfaces (all versions)
	results = append(results, patchAdminAddresses(padDir))

	// Patch 7: Force the container's runtime/admin ports (all versions). The
	// generated config inherits the project's configured ports, which are 0 when
	// unset — the runtime rejects port 0 ("HTTP Port number 0 is not between 1
	// and 65535") and won't start. The compose mapping expects 8080/8090.
	results = append(results, patchRuntimePorts(padDir))

	return results
}

// patchRuntimePorts forces the runtime HTTP port to 8080 and the admin port to
// 8090 in the loaded config (etc/Default), matching the docker-compose port
// mapping. It appends after the includes so it wins over the project's
// configuration, which may leave the ports at 0 — the runtime rejects port 0 and
// crashes on startup.
func patchRuntimePorts(padDir string) PatchResult {
	desc := "Set runtime/admin ports (avoid port 0 startup crash)"
	configPath := filepath.Join(padDir, "etc", "Default")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return PatchResult{Description: desc, Status: "skipped"}
	}
	if strings.Contains(string(content), "# mxcli: runtime/admin ports") {
		return PatchResult{Description: desc, Status: "skipped"}
	}
	patch := "\n# mxcli: runtime/admin ports (container mapping expects 8080/8090; project config may be 0)\nruntime.http { port = 8080 }\nadmin { port = 8090 }\n"
	if err := os.WriteFile(configPath, append(content, []byte(patch)...), 0644); err != nil {
		return PatchResult{Description: desc, Status: "error", Error: fmt.Errorf("writing %s: %w", configPath, err)}
	}
	return PatchResult{Description: desc, Status: "applied"}
}

// patchStartPermissions ensures bin/start has execute permission.
func patchStartPermissions(padDir string) PatchResult {
	startPath := filepath.Join(padDir, "bin", "start")
	info, err := os.Stat(startPath)
	if err != nil {
		return PatchResult{
			Description: "Set bin/start execute permission",
			Status:      "skipped",
		}
	}

	// Check if already executable
	if info.Mode()&0111 != 0 {
		return PatchResult{
			Description: "Set bin/start execute permission",
			Status:      "skipped",
		}
	}

	if err := os.Chmod(startPath, 0755); err != nil {
		return PatchResult{
			Description: "Set bin/start execute permission",
			Status:      "error",
			Error:       fmt.Errorf("chmod bin/start: %w", err),
		}
	}

	return PatchResult{
		Description: "Set bin/start execute permission",
		Status:      "applied",
	}
}

// patchDockerfileCMD replaces start.sh with start in the Dockerfile CMD line.
func patchDockerfileCMD(padDir string) PatchResult {
	dockerfilePath := filepath.Join(padDir, "Dockerfile")
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return PatchResult{
			Description: "Fix Dockerfile CMD (start.sh -> start)",
			Status:      "skipped",
		}
	}

	original := string(content)
	if !strings.Contains(original, "start.sh") {
		return PatchResult{
			Description: "Fix Dockerfile CMD (start.sh -> start)",
			Status:      "skipped",
		}
	}

	updated := strings.ReplaceAll(original, "start.sh", "start")
	if err := os.WriteFile(dockerfilePath, []byte(updated), 0644); err != nil {
		return PatchResult{
			Description: "Fix Dockerfile CMD (start.sh -> start)",
			Status:      "error",
			Error:       fmt.Errorf("writing Dockerfile: %w", err),
		}
	}

	return PatchResult{
		Description: "Fix Dockerfile CMD (start.sh -> start)",
		Status:      "applied",
	}
}

// cmdConfigArgRegex matches CMD lines that pass a config argument to ./bin/start.
// e.g., CMD ["./bin/start", "etc/Default"] or CMD ["./bin/start", "etc/Production"]
var cmdConfigArgRegex = regexp.MustCompile(`(?m)(CMD\s+\["[^"]*start")\s*,\s*"etc/[^"]*"\s*\]`)

// patchDockerfileCMDConfigArg removes config file arguments from the CMD line.
// The bin/start script uses CONFIG_FILES="$CONFIG_FILES $1" which produces a
// leading space (" etc/Default") that the runtime rejects. Removing the arg
// lets bin/start use its own DEFAULT_CONF="etc/Default" variable.
func patchDockerfileCMDConfigArg(padDir string) PatchResult {
	dockerfilePath := filepath.Join(padDir, "Dockerfile")
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return PatchResult{
			Description: "Remove config arg from Dockerfile CMD",
			Status:      "skipped",
		}
	}

	original := string(content)
	if !cmdConfigArgRegex.MatchString(original) {
		return PatchResult{
			Description: "Remove config arg from Dockerfile CMD",
			Status:      "skipped",
		}
	}

	updated := cmdConfigArgRegex.ReplaceAllString(original, "${1}]")
	if err := os.WriteFile(dockerfilePath, []byte(updated), 0644); err != nil {
		return PatchResult{
			Description: "Remove config arg from Dockerfile CMD",
			Status:      "error",
			Error:       fmt.Errorf("writing Dockerfile: %w", err),
		}
	}

	return PatchResult{
		Description: "Remove config arg from Dockerfile CMD",
		Status:      "applied",
	}
}

// openjdkRegex matches FROM lines using deprecated openjdk images.
var openjdkRegex = regexp.MustCompile(`(?m)^(FROM\s+)openjdk:21\S*`)

// patchBaseImage replaces deprecated openjdk:21 with eclipse-temurin:21-jre.
func patchBaseImage(padDir string) PatchResult {
	dockerfilePath := filepath.Join(padDir, "Dockerfile")
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return PatchResult{
			Description: "Replace deprecated openjdk base image",
			Status:      "skipped",
		}
	}

	original := string(content)
	if !openjdkRegex.MatchString(original) {
		return PatchResult{
			Description: "Replace deprecated openjdk base image",
			Status:      "skipped",
		}
	}

	updated := openjdkRegex.ReplaceAllString(original, "${1}eclipse-temurin:21-jre")
	if err := os.WriteFile(dockerfilePath, []byte(updated), 0644); err != nil {
		return PatchResult{
			Description: "Replace deprecated openjdk base image",
			Status:      "error",
			Error:       fmt.Errorf("writing Dockerfile: %w", err),
		}
	}

	return PatchResult{
		Description: "Replace deprecated openjdk base image",
		Status:      "applied",
	}
}

// patchHealthcheck inserts a HEALTHCHECK instruction before the CMD line if not already present.
func patchHealthcheck(padDir string) PatchResult {
	dockerfilePath := filepath.Join(padDir, "Dockerfile")
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return PatchResult{
			Description: "Add HEALTHCHECK instruction",
			Status:      "skipped",
		}
	}

	original := string(content)
	if strings.Contains(original, "HEALTHCHECK") {
		return PatchResult{
			Description: "Add HEALTHCHECK instruction",
			Status:      "skipped",
		}
	}

	healthcheck := "HEALTHCHECK --interval=15s --timeout=5s --start-period=30s --retries=3 \\\n  CMD curl -f http://localhost:8080/ || exit 1\n\n"

	// Insert before CMD line
	lines := strings.Split(original, "\n")
	var result []string
	inserted := false
	for _, line := range lines {
		if !inserted && strings.HasPrefix(strings.TrimSpace(line), "CMD ") {
			result = append(result, healthcheck+line)
			inserted = true
		} else {
			result = append(result, line)
		}
	}

	if !inserted {
		// No CMD found, append at end
		result = append(result, "", healthcheck)
	}

	if err := os.WriteFile(dockerfilePath, []byte(strings.Join(result, "\n")), 0644); err != nil {
		return PatchResult{
			Description: "Add HEALTHCHECK instruction",
			Status:      "error",
			Error:       fmt.Errorf("writing Dockerfile: %w", err),
		}
	}

	return PatchResult{
		Description: "Add HEALTHCHECK instruction",
		Status:      "applied",
	}
}

// patchAdminAddresses appends admin.addresses = ["*"] to the PAD config file
// so the admin API (port 8090) binds to all interfaces inside the container.
// Without this, the admin API only listens on localhost and is unreachable
// from outside the container even when the port is mapped.
func patchAdminAddresses(padDir string) PatchResult {
	configPath := filepath.Join(padDir, "etc", "Default")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return PatchResult{
			Description: "Bind admin API to all interfaces",
			Status:      "skipped",
		}
	}

	if strings.Contains(string(content), `addresses = ["*"]`) {
		return PatchResult{
			Description: "Bind admin API to all interfaces",
			Status:      "skipped",
		}
	}

	patch := "\n# mxcli: bind admin API to all interfaces for dev access\nadmin.addresses = [\"*\"]\n"
	if err := os.WriteFile(configPath, append(content, []byte(patch)...), 0644); err != nil {
		return PatchResult{
			Description: "Bind admin API to all interfaces",
			Status:      "error",
			Error:       fmt.Errorf("writing %s: %w", configPath, err),
		}
	}

	return PatchResult{
		Description: "Bind admin API to all interfaces",
		Status:      "applied",
	}
}
