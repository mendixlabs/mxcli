// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/docker"
)

// RunOptions configures the test runner.
type RunOptions struct {
	// ProjectPath is the path to the .mpr file.
	ProjectPath string

	// TestFiles is the list of test file paths to run.
	TestFiles []string

	// SkipBuild skips the MxBuild step (reuse existing deployment).
	SkipBuild bool

	// Timeout for runtime startup and test execution.
	Timeout time.Duration

	// JUnitOutput is the path for JUnit XML output (empty = no file output).
	JUnitOutput string

	// Verbose shows all runtime log output.
	Verbose bool

	// Color enables colored console output.
	Color bool

	// Stdout for output messages.
	Stdout io.Writer

	// Stderr for error output.
	Stderr io.Writer
}

// Run executes the test suite using the after-startup pattern:
// 1. Parse test files
// 2. Generate TestRunner microflow
// 3. Inject into project, set as after-startup
// 4. Build and restart runtime
// 5. Parse logs for results
// 6. Cleanup (restore original settings)
// 7. Output results
func Run(opts RunOptions) (*SuiteResult, error) {
	w := opts.Stdout
	if w == nil {
		w = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	// Step 1: Parse test files
	fmt.Fprintln(w, "Parsing test files...")
	suite, err := parseTestFiles(opts.TestFiles)
	if err != nil {
		return nil, fmt.Errorf("parsing test files: %w", err)
	}
	fmt.Fprintf(w, "  Found %d test(s) in %d file(s)\n", len(suite.Tests), len(opts.TestFiles))

	if len(suite.Tests) == 0 {
		return nil, fmt.Errorf("no tests found in the provided files")
	}

	// Step 2: Generate TestRunner microflow MDL
	fmt.Fprintln(w, "Generating test runner microflow...")
	runnerMDL := GenerateTestRunner(suite)

	if opts.Verbose {
		fmt.Fprintln(w, "--- Generated MDL ---")
		fmt.Fprintln(w, runnerMDL)
		fmt.Fprintln(w, "--- End MDL ---")
	}

	// Step 3: Save original settings and inject test runner
	fmt.Fprintln(w, "Injecting test runner into project...")
	origAfterStartup, err := getAfterStartup(opts.ProjectPath)
	if err != nil {
		fmt.Fprintf(w, "  Warning: could not read original after-startup setting: %v\n", err)
	}

	// Write the runner MDL to a temp file and execute it
	tmpFile, err := os.CreateTemp("", "mxtest-runner-*.mdl")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(runnerMDL); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("writing runner MDL: %w", err)
	}
	tmpFile.Close()

	// Execute the MDL to create the TestRunner microflow
	if err := execMxcli(opts.ProjectPath, "exec", tmpPath, "-p", opts.ProjectPath); err != nil {
		return nil, fmt.Errorf("injecting test runner: %w", err)
	}

	// Set security OFF for testing
	if err := execMxcliCmd(opts.ProjectPath, "ALTER PROJECT SECURITY LEVEL OFF"); err != nil {
		fmt.Fprintf(w, "  Warning: could not set security OFF: %v\n", err)
	}

	// Set after-startup microflow
	if err := execMxcliCmd(opts.ProjectPath, "ALTER SETTINGS MODEL AfterStartupMicroflow = 'MxTest.TestRunner'"); err != nil {
		return nil, fmt.Errorf("setting after-startup: %w", err)
	}
	fmt.Fprintln(w, "  After-startup set to MxTest.TestRunner")

	// Step 4: Build and restart
	dockerDir := filepath.Join(filepath.Dir(opts.ProjectPath), ".docker")
	if err := ensureDockerStack(opts.ProjectPath, dockerDir, w); err != nil {
		cleanup(opts.ProjectPath, origAfterStartup, w)
		return nil, fmt.Errorf("docker init: %w", err)
	}

	if !opts.SkipBuild {
		fmt.Fprintln(w, "Building project...")
		if err := execMxcli(opts.ProjectPath, "docker", "build", "-p", opts.ProjectPath, "--skip-check"); err != nil {
			cleanup(opts.ProjectPath, origAfterStartup, w)
			return nil, fmt.Errorf("docker build: %w", err)
		}
	}

	fmt.Fprintln(w, "Restarting runtime...")
	// Stop existing containers
	runCompose(dockerDir, "down")
	// Start fresh
	if err := runCompose(dockerDir, "up", "--detach", "--force-recreate"); err != nil {
		cleanup(opts.ProjectPath, origAfterStartup, w)
		return nil, fmt.Errorf("docker up: %w", err)
	}

	// Step 5: Wait for runtime and capture logs
	fmt.Fprintf(w, "Waiting for test execution (timeout: %s)...\n", timeout)
	logOutput, err := captureRuntimeLogs(dockerDir, timeout, w, opts.Verbose)
	if err != nil {
		cleanup(opts.ProjectPath, origAfterStartup, w)
		return nil, fmt.Errorf("runtime execution: %w", err)
	}

	// Step 6: Parse results from logs
	fmt.Fprintln(w, "Parsing test results...")
	result := ParseLogResults(strings.NewReader(logOutput), suite)

	// Step 7: Cleanup
	fmt.Fprintln(w, "Cleaning up...")
	cleanup(opts.ProjectPath, origAfterStartup, w)

	// Step 8: Output results
	PrintResults(w, result, opts.Color)

	// Write JUnit XML if requested
	if opts.JUnitOutput != "" {
		f, err := os.Create(opts.JUnitOutput)
		if err != nil {
			return result, fmt.Errorf("creating JUnit output: %w", err)
		}
		defer f.Close()
		if err := WriteJUnitXML(f, result); err != nil {
			return result, fmt.Errorf("writing JUnit XML: %w", err)
		}
		fmt.Fprintf(w, "JUnit XML written to: %s\n", opts.JUnitOutput)
	}

	return result, nil
}

// ListTests parses test files and prints the test names without executing.
func ListTests(files []string, w io.Writer) error {
	suite, err := parseTestFiles(files)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Found %d test(s):\n", len(suite.Tests))
	for _, tc := range suite.Tests {
		fmt.Fprintf(w, "  %s: %s\n", tc.ID, tc.Name)
		if len(tc.Expects) > 0 {
			for _, exp := range tc.Expects {
				fmt.Fprintf(w, "    @expect %s %s %s\n", exp.Variable, exp.Operator, exp.Value)
			}
		}
		if tc.Throws != "" {
			fmt.Fprintf(w, "    @throws '%s'\n", tc.Throws)
		}
		for _, v := range tc.Verify {
			fmt.Fprintf(w, "    @verify %s\n", v)
		}
	}
	return nil
}

// parseTestFiles parses one or more test files or directories.
func parseTestFiles(paths []string) (*TestSuite, error) {
	combined := &TestSuite{
		Name: "mxtest",
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}

		if info.IsDir() {
			dirSuite, err := ParseTestDir(path)
			if err != nil {
				return nil, err
			}
			if combined.Name == "mxtest" && dirSuite.Name != "" {
				combined.Name = dirSuite.Name
			}
			combined.Tests = append(combined.Tests, dirSuite.Tests...)
		} else {
			fileSuite, err := ParseTestFile(path)
			if err != nil {
				return nil, err
			}
			if combined.Name == "mxtest" && fileSuite.Name != "" {
				combined.Name = fileSuite.Name
			}
			combined.Tests = append(combined.Tests, fileSuite.Tests...)
		}
	}

	// Re-number test IDs to be globally unique
	for i := range combined.Tests {
		combined.Tests[i].ID = fmt.Sprintf("test_%d", i+1)
	}

	return combined, nil
}

// getAfterStartup reads the current after-startup microflow setting.
func getAfterStartup(projectPath string) (string, error) {
	mxcliPath, err := findMxcli()
	if err != nil {
		return "", err
	}

	cmd := exec.Command(mxcliPath, "-p", projectPath, "-c", "DESCRIBE SETTINGS")
	cmd.Env = append(os.Environ(), "MXCLI_QUIET=1")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse output for AfterStartupMicroflow
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "AfterStartupMicroflow") {
			// Extract the value from: AfterStartupMicroflow = 'Module.Name'
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, "'\"")
				val = strings.TrimSuffix(val, ";")
				val = strings.TrimSpace(val)
				return val, nil
			}
		}
	}

	return "", nil
}

// cleanup restores original project settings after testing.
func cleanup(projectPath, origAfterStartup string, w io.Writer) {
	// Restore original after-startup
	if origAfterStartup != "" {
		cmd := fmt.Sprintf("ALTER SETTINGS MODEL AfterStartupMicroflow = '%s'", origAfterStartup)
		if err := execMxcliCmd(projectPath, cmd); err != nil {
			fmt.Fprintf(w, "  Warning: could not restore after-startup: %v\n", err)
		}
	} else {
		if err := execMxcliCmd(projectPath, "ALTER SETTINGS MODEL AfterStartupMicroflow = ''"); err != nil {
			fmt.Fprintf(w, "  Warning: could not clear after-startup: %v\n", err)
		}
	}

	// Restore security level
	if err := execMxcliCmd(projectPath, "ALTER PROJECT SECURITY LEVEL PRODUCTION"); err != nil {
		fmt.Fprintf(w, "  Warning: could not restore security: %v\n", err)
	}

	// Drop the test runner microflow
	execMxcliCmd(projectPath, "DROP MICROFLOW MxTest.TestRunner")
}

// captureRuntimeLogs tails the docker compose logs, waiting for MXTEST:END or timeout.
// Returns the captured log output.
func captureRuntimeLogs(dockerDir string, timeout time.Duration, w io.Writer, verbose bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, docker.ContainerCLI(), "compose", "logs", "--follow", "--no-log-prefix", "--since", "1s", "mendix")
	cmd.Dir = dockerDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("creating log pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting log follow: %w", err)
	}

	var logBuf bytes.Buffer
	scanner := bufio.NewScanner(stdout)
	done := false
	runtimeFailed := false
	var failMsg string

	for scanner.Scan() {
		line := scanner.Text()
		logBuf.WriteString(line)
		logBuf.WriteString("\n")

		if verbose {
			fmt.Fprintln(w, line)
		}

		// Check for test completion
		if strings.Contains(line, "MXTEST:END:") {
			done = true
			cancel()
			break
		}

		// Check for after-startup completion (tests ran)
		if strings.Contains(line, "Successfully ran after-startup-action") {
			done = true
			cancel()
			break
		}

		// Check for runtime failure
		if strings.Contains(line, "Error starting runtime") ||
			strings.Contains(line, "Critical error") ||
			strings.Contains(line, "After startup microflow should return a boolean") {
			runtimeFailed = true
			failMsg = line
			cancel()
			break
		}

		// Also catch after-startup failures
		if strings.Contains(line, "after-startup-action failed") {
			// The after-startup failed — tests may have partially run
			done = true
			cancel()
			break
		}
	}

	cmd.Process.Kill()
	cmd.Wait()

	if runtimeFailed {
		return logBuf.String(), fmt.Errorf("runtime failed: %s", failMsg)
	}

	if !done && ctx.Err() == context.DeadlineExceeded {
		return logBuf.String(), fmt.Errorf("timeout after %s waiting for test completion", timeout)
	}

	return logBuf.String(), nil
}

// execMxcli runs an mxcli subcommand.
func execMxcli(projectPath string, args ...string) error {
	mxcliPath, err := findMxcli()
	if err != nil {
		return err
	}

	cmd := exec.Command(mxcliPath, args...)
	cmd.Env = append(os.Environ(), "MXCLI_QUIET=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// execMxcliCmd runs an MDL command via mxcli -p ... -c "...".
func execMxcliCmd(projectPath, mdlCmd string) error {
	mxcliPath, err := findMxcli()
	if err != nil {
		return err
	}

	cmd := exec.Command(mxcliPath, "-p", projectPath, "-c", mdlCmd)
	cmd.Env = append(os.Environ(), "MXCLI_QUIET=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// findMxcli locates the mxcli binary.
func findMxcli() (string, error) {
	// First check if we're running as mxcli ourselves
	exe, err := os.Executable()
	if err == nil {
		return exe, nil
	}

	// Look in PATH
	path, err := exec.LookPath("mxcli")
	if err == nil {
		return path, nil
	}

	// Look in common locations
	for _, p := range []string{"./mxcli", "./bin/mxcli", "../bin/mxcli"} {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs, nil
		}
	}

	return "", fmt.Errorf("mxcli binary not found (ensure it's in PATH or current directory)")
}

func ensureDockerStack(projectPath, dockerDir string, w io.Writer) error {
	composePath := filepath.Join(dockerDir, "docker-compose.yml")
	if _, err := os.Stat(composePath); err == nil {
		return nil
	}
	return docker.Init(docker.InitOptions{
		ProjectPath: projectPath,
		OutputDir:   dockerDir,
		Stdout:      w,
	})
}

// runCompose executes a docker compose command in the given directory.
func runCompose(dockerDir string, args ...string) error {
	cmd := exec.Command(docker.ContainerCLI(), append([]string{"compose"}, args...)...)
	cmd.Dir = dockerDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
