// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const baselineVersion = 1

// MxBaseline is the saved mxcheck state. The lint stage keeps lint's own
// baseline file (so a `lint --baseline` file stays valid); only mxcheck needs
// this one.
type MxBaseline struct {
	Version int `json:"version"`
	// Checker names the tool that produced the findings and MxVersion its version
	// when known: codes and staging differ between checkers and versions, so a
	// baseline is only compared with a run of the same one.
	Checker   string    `json:"checker"`
	MxVersion string    `json:"mxVersion,omitempty"`
	Findings  []Finding `json:"findings"`
}

// WriteMxBaseline records findings at path, sorted so that re-recording an
// unchanged project rewrites the same bytes and a committed baseline diffs only
// where findings changed.
func WriteMxBaseline(path, checker, mxVersion string, findings []Finding) error {
	fs := append([]Finding{}, findings...)
	SortFindings(fs)
	data, err := json.MarshalIndent(MxBaseline{Version: baselineVersion, Checker: checker, MxVersion: mxVersion, Findings: fs}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// LoadMxBaseline reads a baseline. It returns (nil, nil) when there is none, so a
// caller can say "unbaselined" instead of failing.
func LoadMxBaseline(path string) (*MxBaseline, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var b MxBaseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("read baseline %s: %w", path, err)
	}
	if b.Version != baselineVersion {
		return nil, fmt.Errorf("baseline %s has version %d; this mxcli reads %d", path, b.Version, baselineVersion)
	}
	return &b, nil
}

// CheckCompatible refuses to compare findings with a baseline of another checker
// or mx version: that diff would report every renumbered code as new.
func (b *MxBaseline) CheckCompatible(checker, mxVersion string) error {
	if b.Checker != checker {
		return fmt.Errorf("the baseline was recorded with %s, this run used %s; re-record it with --update-baseline", b.Checker, checker)
	}
	if b.MxVersion != "" && mxVersion != "" && b.MxVersion != mxVersion {
		return fmt.Errorf("the baseline was recorded with mx %s, this run used mx %s; re-record it with --update-baseline", b.MxVersion, mxVersion)
	}
	return nil
}
