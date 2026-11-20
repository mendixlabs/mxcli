// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// testEndpointGitignoreEntry ignores the Java source `run --test-endpoint`
// generates. RegisterTestEndpoint.java sits in the working tree for as long as
// the app hosts the endpoint, and a `git add -A` in that window committed it
// (ako/mxcli#2033). Removing the endpoint deletes the folder again.
const testEndpointGitignoreEntry = "\n# mxcli test endpoint: Java source generated while 'mxcli run --test-endpoint'\n" +
	"# hosts it, removed when the run stops. Never commit it.\n/javasource/mxtest/\n"

// ensureTestEndpointIgnored makes the test endpoint's generated source, and
// .mxcli/ (which holds the endpoint's handshake and token), git-ignored before
// the endpoint is installed. A project without a .gitignore gets the one
// `mxcli init` writes; an existing one keeps its lines and gains what is
// missing.
func ensureTestEndpointIgnored(projectPath string) error {
	path := filepath.Join(filepath.Dir(projectPath), ".gitignore")
	body, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		if err := os.WriteFile(path, []byte(mendixGitignore), 0o644); err != nil {
			return fmt.Errorf("creating %s: %w", path, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if !mentionsTestEndpointDir(string(body)) {
		content := string(body)
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if err := os.WriteFile(path, []byte(content+testEndpointGitignoreEntry), 0o644); err != nil {
			return fmt.Errorf("appending to %s: %w", path, err)
		}
	}
	_, err = ensureStoreIgnored(projectPath)
	return err
}

// mentionsTestEndpointDir reports whether a .gitignore already names the test
// endpoint's source folder, in any of the usual spellings.
func mentionsTestEndpointDir(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		switch strings.TrimSpace(line) {
		case "/javasource/mxtest/", "/javasource/mxtest", "javasource/mxtest/", "javasource/mxtest":
			return true
		}
	}
	return false
}
