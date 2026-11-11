// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Default ports used by the Mendix runtime inside the container.
// These are fixed regardless of any port offset applied to host-side mappings.
const (
	containerAdminPort = 8090
	containerAppPort   = 8080
)

// M2EEOptions configures connection to the M2EE admin API.
type M2EEOptions struct {
	// Host is the hostname of the Mendix admin API (default: localhost).
	Host string

	// Port is the admin API port (default: 8090).
	Port int

	// Token is the M2EE admin password for authentication.
	Token string

	// ProjectPath is the path to the .mpr file (used to find .docker/.env).
	ProjectPath string

	// Direct bypasses docker exec and connects to the admin API directly.
	Direct bool

	// Timeout is the HTTP client timeout (default: 10s).
	Timeout time.Duration
}

// ErrAdminAuth marks the admin API refusing the credentials (HTTP 401/403).
// Against a `mxcli run --local` that usually means the credentials were not
// that run's — its .mxcli/run-local.json was missing — rather than a wrong
// password, so a caller that knows the project can say so (ako/mxcli#1542).
var ErrAdminAuth = errors.New("admin API refused the credentials")

// adminError keeps an admin-API failure's text while letting errors.Is see
// what kind of failure it was.
type adminError struct {
	msg  string
	kind error
	// timedOut marks a request the admin API accepted and did not answer in
	// time, as opposed to one it never received.
	timedOut bool
}

// isAdminTimeout reports whether err is an admin call that ran out of time.
func isAdminTimeout(err error) bool {
	var ae *adminError
	return errors.As(err, &ae) && ae.timedOut
}

func (e *adminError) Error() string        { return e.msg }
func (e *adminError) Is(target error) bool { return target == e.kind }

func adminAuthFailed(status int) error {
	return &adminError{kind: ErrAdminAuth,
		msg: fmt.Sprintf("authentication failed (HTTP %d) -- check the admin password (--token or M2EE_ADMIN_PASS)", status)}
}

func adminUnreachable(msg string) error {
	return &adminError{kind: ErrAdminUnreachable, msg: msg}
}

// M2EEResponse is the parsed envelope from the admin API.
type M2EEResponse struct {
	Result      int             `json:"result"`
	Cause       string          `json:"cause"`
	Message     string          `json:"message"`
	RawFeedback json.RawMessage `json:"feedback"`
}

// Feedback decodes the raw feedback JSON into a map.
// Returns nil map if feedback is empty or not a JSON object.
func (r *M2EEResponse) Feedback() map[string]any {
	if len(r.RawFeedback) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(r.RawFeedback, &m); err != nil {
		return nil
	}
	return m
}

// CallM2EE sends an action to the admin API and returns the parsed response.
// It supports both direct HTTP and docker-exec transport modes.
func CallM2EE(opts M2EEOptions, action string, params map[string]any) (*M2EEResponse, error) {
	if err := resolveM2EEDefaults(&opts); err != nil {
		return nil, err
	}

	// Use docker exec mode when we have a project path and Direct is not forced
	if !opts.Direct && opts.ProjectPath != "" {
		dockerDir := filepath.Join(filepath.Dir(opts.ProjectPath), ".docker")
		composePath := filepath.Join(dockerDir, "docker-compose.yml")
		if _, err := os.Stat(composePath); err == nil {
			return callM2EEViaDocker(opts, dockerDir, action, params)
		}
	}

	return callM2EEDirect(opts, action, params)
}

// callM2EEDirect sends the request via direct HTTP to the admin API.
func callM2EEDirect(opts M2EEOptions, action string, params map[string]any) (*M2EEResponse, error) {
	url := fmt.Sprintf("http://%s:%d/", opts.Host, opts.Port)

	body := map[string]any{
		"action": action,
	}
	if params != nil {
		body["params"] = params
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-M2EE-Authentication", m2eeAuthHeader(opts.Token))

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		if isTimeout(err) {
			// It accepted the request and is still working on it: "cannot
			// connect ... is the app running?" sent a user hunting for a dead
			// process while a slow after-startup microflow ran (#1818).
			return nil, &adminError{kind: ErrAdminUnreachable, timedOut: true,
				msg: fmt.Sprintf("Mendix admin API at %s:%d did not answer %q within %s", opts.Host, opts.Port, action, timeout)}
		}
		return nil, adminUnreachable(fmt.Sprintf("cannot connect to Mendix admin API at %s:%d -- is the app running? Start with 'mxcli docker up'", opts.Host, opts.Port))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, adminAuthFailed(resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d from admin API", resp.StatusCode)
	}

	var m2eeResp M2EEResponse
	if err := json.NewDecoder(resp.Body).Decode(&m2eeResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &m2eeResp, nil
}

// callM2EEViaDocker runs the request inside the Mendix container using
// "docker compose exec". This bypasses DinD network issues where the admin API
// (port 8090) binds to 127.0.0.1 inside the container and is unreachable from
// the devcontainer host.
func callM2EEViaDocker(opts M2EEOptions, dockerDir string, action string, params map[string]any) (*M2EEResponse, error) {
	body := map[string]any{
		"action": action,
	}
	if params != nil {
		body["params"] = params
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	authHeader := m2eeAuthHeader(opts.Token)

	// Use curl inside the container to hit the admin API.
	// Always use the container-internal port (8090), not opts.Port which is the
	// host-side port (may differ when port offset is applied).
	curlCmd := fmt.Sprintf(
		"curl -sf -X POST http://localhost:%d/ -H 'Content-Type: application/json' -H 'X-M2EE-Authentication: %s' -d '%s'",
		containerAdminPort, authHeader, strings.ReplaceAll(string(bodyBytes), "'", "'\\''"),
	)

	composePath := filepath.Join(dockerDir, "docker-compose.yml")
	cmd := exec.Command(ContainerCLI(), "compose", "-f", composePath, "exec", "-T", "mendix", "sh", "-c", curlCmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		// docker compose reports a stopped/absent container as
		// `service "mendix" is not running` / `no such service`. Translate that
		// into an actionable hint instead of surfacing the raw compose error.
		if strings.Contains(stderrStr, "is not running") || strings.Contains(stderrStr, "no such service") {
			return nil, fmt.Errorf("Mendix container is not running -- start it with 'mxcli docker up'")
		}
		if stderrStr != "" {
			return nil, fmt.Errorf("docker compose exec failed: %s", stderrStr)
		}
		return nil, fmt.Errorf("cannot execute in Mendix container -- is the app running? Start with 'mxcli docker up'")
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("empty response from Mendix admin API -- is the app running?")
	}

	var m2eeResp M2EEResponse
	if err := json.NewDecoder(&stdout).Decode(&m2eeResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &m2eeResp, nil
}

// devPreviewOQLPath is the Mendix 11.11+ REST endpoint for OQL preview. It
// replaced the legacy M2EE action dispatch (POST / with {"action","params"}):
// the request body is the params object directly and the response is
// {"data":[...]} with no feedback envelope. Auth is unchanged
// (X-M2EE-Authentication: base64(password)).
const devPreviewOQLPath = "/dev/preview_execute_oql"

// errDevEndpointNotFound signals that the runtime does not expose the 11.11+
// /dev/preview_execute_oql endpoint (HTTP 404), so the caller should fall back
// to the legacy preview_execute_oql action.
var errDevEndpointNotFound = errors.New("dev preview endpoint not found")

// previewOQLDev calls the 11.11+ REST endpoint and returns the raw response body
// (a {"data":[...]} object). It returns errDevEndpointNotFound when the runtime
// responds 404, so the caller can fall back to the legacy action API. Transport
// (direct HTTP vs docker-exec) mirrors CallM2EE.
func previewOQLDev(opts M2EEOptions, params map[string]any) (json.RawMessage, error) {
	if err := resolveM2EEDefaults(&opts); err != nil {
		return nil, err
	}

	bodyBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	if !opts.Direct && opts.ProjectPath != "" {
		dockerDir := filepath.Join(filepath.Dir(opts.ProjectPath), ".docker")
		composePath := filepath.Join(dockerDir, "docker-compose.yml")
		if _, err := os.Stat(composePath); err == nil {
			return previewOQLDevDocker(opts, dockerDir, bodyBytes)
		}
	}

	return previewOQLDevDirect(opts, bodyBytes)
}

// previewOQLDevDirect POSTs the raw params body to the dev endpoint over HTTP.
func previewOQLDevDirect(opts M2EEOptions, bodyBytes []byte) (json.RawMessage, error) {
	url := fmt.Sprintf("http://%s:%d%s", opts.Host, opts.Port, devPreviewOQLPath)

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-M2EE-Authentication", m2eeAuthHeader(opts.Token))

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, adminUnreachable(fmt.Sprintf("cannot connect to Mendix admin API at %s:%d -- is the app running? Start with 'mxcli docker up'", opts.Host, opts.Port))
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errDevEndpointNotFound
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, adminAuthFailed(resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("unexpected HTTP %d from admin API", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return respBody, nil
}

// previewOQLDevDocker POSTs to the dev endpoint via "docker compose exec" + curl.
// curl is run without -f and with -w so the trailing HTTP status survives, which
// lets us detect 404 (pre-11.11) and fall back.
func previewOQLDevDocker(opts M2EEOptions, dockerDir string, bodyBytes []byte) (json.RawMessage, error) {
	authHeader := m2eeAuthHeader(opts.Token)

	curlCmd := fmt.Sprintf(
		"curl -s -w '\\n%%{http_code}' -X POST http://localhost:%d%s -H 'Content-Type: application/json' -H 'X-M2EE-Authentication: %s' -d '%s'",
		containerAdminPort, devPreviewOQLPath, authHeader, strings.ReplaceAll(string(bodyBytes), "'", "'\\''"),
	)

	composePath := filepath.Join(dockerDir, "docker-compose.yml")
	cmd := exec.Command(ContainerCLI(), "compose", "-f", composePath, "exec", "-T", "mendix", "sh", "-c", curlCmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("docker compose exec failed: %s", stderrStr)
		}
		return nil, fmt.Errorf("cannot execute in Mendix container -- is the app running? Start with 'mxcli docker up'")
	}

	body, status, ok := splitCurlStatus(stdout.Bytes())
	if !ok || status == 0 {
		return nil, adminUnreachable("cannot connect to Mendix admin API -- is the app running? Start with 'mxcli docker up'")
	}
	switch status {
	case http.StatusNotFound:
		return nil, errDevEndpointNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, adminAuthFailed(status)
	case http.StatusOK:
		return body, nil
	default:
		return nil, fmt.Errorf("unexpected HTTP %d from admin API", status)
	}
}

// splitCurlStatus splits curl output produced with -w '\n%{http_code}' into the
// response body and the trailing HTTP status code. ok is false when no numeric
// status line is present (e.g. a connection failure produced no output).
func splitCurlStatus(out []byte) (body []byte, status int, ok bool) {
	out = bytes.TrimRight(out, "\n")
	idx := bytes.LastIndexByte(out, '\n')
	code, err := strconv.Atoi(strings.TrimSpace(string(out[idx+1:])))
	if err != nil {
		return nil, 0, false
	}
	if idx < 0 {
		return nil, code, true
	}
	return out[:idx], code, true
}

// resolveM2EEDefaults fills missing M2EEOptions fields from env vars and .env file.
// Priority: explicit value > environment variable > .docker/.env file > default.
func resolveM2EEDefaults(opts *M2EEOptions) error {
	// Load .env values if we can find the .docker directory
	envVals := make(map[string]string)
	if opts.ProjectPath != "" {
		dockerDir := filepath.Join(filepath.Dir(opts.ProjectPath), ".docker")
		envPath := filepath.Join(dockerDir, ".env")
		if parsed, err := parseEnvFile(envPath); err == nil {
			envVals = parsed
		}
	}

	// Host: flag > env > default
	if opts.Host == "" {
		if v := os.Getenv("MCP_OQL_MENDIX_HOST"); v != "" {
			opts.Host = v
		} else {
			opts.Host = "localhost"
		}
	}

	// Port: flag > env > .env > default
	if opts.Port == 0 {
		if v := os.Getenv("ADMIN_PORT"); v != "" {
			var port int
			if _, err := fmt.Sscanf(v, "%d", &port); err == nil && port > 0 {
				opts.Port = port
			}
		}
		if opts.Port == 0 {
			if v, ok := envVals["ADMIN_PORT"]; ok {
				var port int
				if _, err := fmt.Sscanf(v, "%d", &port); err == nil && port > 0 {
					opts.Port = port
				}
			}
		}
		if opts.Port == 0 {
			opts.Port = 8090
		}
	}

	// Token: flag > env > .env > local dev default.
	//
	// A `mxcli run --local` app has no .docker/.env and never prints the password
	// it used, so before this fallback `mxcli oql` against a local run failed with
	// "admin password required" (findings #36). The local runtime always boots
	// with defaultLocalAdminPass, and the admin API binds to loopback only, so
	// using it as the last resort makes `mxcli oql` work against `run --local`
	// with no configuration. A docker app that genuinely uses a different password
	// records it in .docker/.env (loaded above); if it somehow doesn't, the wrong
	// password surfaces as a clear "authentication failed" rather than a silent
	// miss.
	if opts.Token == "" {
		if v := os.Getenv("M2EE_ADMIN_PASS"); v != "" {
			opts.Token = v
		} else if v, ok := envVals["M2EE_ADMIN_PASS"]; ok {
			opts.Token = v
		}
	}
	if opts.Token == "" {
		opts.Token = defaultLocalAdminPass
	}

	return nil
}

// M2EEError returns the error message from an M2EEResponse, or "" if result==0.
func (r *M2EEResponse) M2EEError() string {
	if r.Result == 0 {
		return ""
	}
	msg := r.Cause
	if msg == "" {
		msg = r.Message
	}
	if msg == "" {
		msg = "unknown error"
	}
	return msg
}

// m2eeAuthHeader returns the base64-encoded M2EE authentication token.
// The M2EE admin API expects X-M2EE-Authentication: base64(password).
func m2eeAuthHeader(password string) string {
	return base64.StdEncoding.EncodeToString([]byte(password))
}

// parseEnvFile reads a KEY=VALUE file, skipping comments and blank lines.
func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseEnvReader(f)
}

// parseEnvReader reads KEY=VALUE pairs from a reader.
func parseEnvReader(r io.Reader) (map[string]string, error) {
	result := make(map[string]string)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first =
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Strip surrounding quotes
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		result[key] = value
	}

	return result, scanner.Err()
}
