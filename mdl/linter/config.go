// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Config holds the lint configuration.
type Config struct {
	// ExcludeModules lists modules to exclude from linting
	ExcludeModules []string `yaml:"excludeModules"`

	// Rules configures individual rules
	Rules map[string]RuleConfigYAML `yaml:"rules"`
}

// RuleConfigYAML is the YAML representation of rule configuration.
type RuleConfigYAML struct {
	Enabled  *bool          `yaml:"enabled"`
	Severity string         `yaml:"severity"`
	Options  map[string]any `yaml:"options"`
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		ExcludeModules: []string{},
		Rules:          make(map[string]RuleConfigYAML),
	}
}

// LoadConfig loads lint configuration from a file.
// Returns default config if file doesn't exist.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// FindConfigFile searches for lint config in standard locations.
// Searches: .claude/lint-config.yaml, lint-config.yaml, .lint-config.yaml
func FindConfigFile(projectDir string) string {
	candidates := []string{
		filepath.Join(projectDir, ".claude", "lint-config.yaml"),
		filepath.Join(projectDir, "lint-config.yaml"),
		filepath.Join(projectDir, ".lint-config.yaml"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// ApplyConfig applies configuration to a linter.
func (cfg *Config) ApplyConfig(l *Linter) {
	for ruleID, ruleCfg := range cfg.Rules {
		config := RuleConfig{
			Enabled: true,
			Options: ruleCfg.Options,
		}

		if ruleCfg.Enabled != nil {
			config.Enabled = *ruleCfg.Enabled
		}

		switch ruleCfg.Severity {
		case "error":
			config.Severity = SeverityError
		case "warning":
			config.Severity = SeverityWarning
		case "info":
			config.Severity = SeverityInfo
		case "hint":
			config.Severity = SeverityHint
		default:
			// Keep default severity from rule
			for _, rule := range l.Rules() {
				if rule.ID() == ruleID {
					config.Severity = rule.DefaultSeverity()
					break
				}
			}
		}

		l.ConfigureRule(ruleID, config)
	}
}

// ParseSeverity parses a severity string.
func ParseSeverity(s string) Severity {
	switch s {
	case "error":
		return SeverityError
	case "warning":
		return SeverityWarning
	case "info":
		return SeverityInfo
	case "hint":
		return SeverityHint
	default:
		return SeverityWarning
	}
}

// Fingerprint identifies the configuration a lint run used: the effective
// excluded modules (the config file's plus any given on the command line) and
// each rule's enablement, severity and options. It is computed from the parsed
// values, not the file's bytes, so a comment or reordering does not change it,
// and a baseline records it to tell a run under another configuration from one
// under the same (ako/mxcli#2455). cfg may be nil.
func (cfg *Config) Fingerprint(cliExcluded []string) string {
	type rule struct {
		Enabled  *bool          `json:"enabled,omitempty"`
		Severity string         `json:"severity,omitempty"`
		Options  map[string]any `json:"options,omitempty"`
	}
	eff := struct {
		Exclude []string        `json:"exclude"`
		Rules   map[string]rule `json:"rules"`
	}{Exclude: []string{}, Rules: map[string]rule{}}
	seen := map[string]bool{}
	add := func(ms []string) {
		for _, m := range ms {
			if !seen[m] {
				seen[m] = true
				eff.Exclude = append(eff.Exclude, m)
			}
		}
	}
	if cfg != nil {
		add(cfg.ExcludeModules)
		for id, r := range cfg.Rules {
			eff.Rules[id] = rule{Enabled: r.Enabled, Severity: r.Severity, Options: r.Options}
		}
	}
	add(cliExcluded)
	sort.Strings(eff.Exclude)
	data, _ := json.Marshal(eff) // map keys are sorted by encoding/json
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}
