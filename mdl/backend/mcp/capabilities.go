// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	_ "embed"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// capabilities.yaml is the version-keyed table half of the capability model
// (ADR-0004): the non-probeable facts (create whitelist, behavioral quirks). Tool
// presence comes from a live tools/list probe, merged in by (*Backend).capabilities.
//
//go:embed capabilities.yaml
var capabilityTableYAML []byte

// Capability keys gated by the backend (must match `key:` in capabilities.yaml).
const (
	capNanoflowCreate      = "nanoflow_create"
	capJavaActionCreate    = "javaaction_create"
	capBusinessEventCreate = "businessevent_create"
	capViewEntityCreate    = "view_entities"
)

// Capability is one authorable/blocked feature, resolved for the connected
// session (project Mendix version + live tool probe).
type Capability struct {
	Key       string
	Feature   string
	Available bool
	Note      string
	// Blocker, when non-empty, says why an otherwise-available feature is off
	// for *this* session (a missing tool, or a probe that could not run). It is
	// session state, not a property of the version — see ADR-0006's Revision.
	Blocker string
}

// Capabilities is the effective capability set for a connected server: the
// table (keyed on the project's Mendix version) merged with the live server
// identity and tool probe. The agent-facing report and the backend's authoring
// gates read from it, so they cannot drift.
//
// It is valid only for the session that produced it: tool presence varies with
// the user's Studio Pro preferences and configured MCP servers, not just with
// versions (ADR-0006 Revision).
type Capabilities struct {
	ServerName     string
	ServerVersion  string
	ProjectVersion string
	// Tools are the Studio Pro tools present, from the live probe.
	Tools []string
	// FederatedTools are tools Studio Pro proxies from MCP servers the user has
	// connected to it (prefixed mcp_<server>_<tool>). Reported for visibility,
	// never gated on: mxcli does not control their contract.
	FederatedTools []string
	// ToolsProbed records whether tools/list actually answered. False means tool
	// presence is unknown, and tool-dependent features fail closed.
	ToolsProbed bool
	Features    []Capability
}

// federatedToolPrefix marks a tool Studio Pro proxies from another MCP server.
// Studio Pro's system prompt: "MCP tools are prefixed mcp_{serverName}_{toolName}".
const federatedToolPrefix = "mcp_"

type capabilityTable struct {
	Features []struct {
		Key       string `yaml:"key"`
		Feature   string `yaml:"feature"`
		Available bool   `yaml:"available"`
		// AvailableSinceMendix is the project's Mendix version ("11.12") from
		// which this feature is authorable. Keyed on the *project* version
		// because the MCP serverInfo.version is frozen at 1.0.0 across releases
		// and cannot discriminate them (ADR-0006 Revision).
		AvailableSinceMendix string `yaml:"available_since_mendix"`
		// RequiresTools are the Studio Pro tools the feature needs. Which tools a
		// feature depends on is not observable, so it lives here; whether they are
		// present is answered only by the live probe.
		RequiresTools []string `yaml:"requires_tools"`
		Note          string   `yaml:"note"`
	} `yaml:"features"`
}

// loadCapabilityTable returns the embedded table. Embedded + validated by
// TestCapabilityTableParses; a parse failure would be a build-time content bug,
// so degrade to empty rather than panic.
func loadCapabilityTable() capabilityTable {
	var t capabilityTable
	_ = yaml.Unmarshal(capabilityTableYAML, &t)
	return t
}

// resolveCapabilities computes the effective feature set for a session.
//
// Three inputs, in order: the table's baseline, the project's Mendix version
// (which can turn a baseline-blocked feature on), and the live tool probe (which
// can turn any feature off). The probe only ever subtracts — a feature mxcli has
// no create path for does not become available because a tool appeared.
func resolveCapabilities(pv *types.ProjectVersion, tools []string, probed bool) []Capability {
	present := make(map[string]bool, len(tools))
	for _, t := range tools {
		present[t] = true
	}
	t := loadCapabilityTable()
	out := make([]Capability, 0, len(t.Features))
	for _, f := range t.Features {
		c := Capability{Key: f.Key, Feature: f.Feature, Available: f.Available, Note: f.Note}
		if !c.Available && f.AvailableSinceMendix != "" && projectVersionAtLeast(pv, f.AvailableSinceMendix) {
			c.Available = true
		}
		// Fail closed: an unavailable tool, or an unknown tool surface, blocks a
		// feature that needs it. A false "no" is the safe direction for a write
		// path — the alternative is failing mid-write against a missing tool.
		if c.Available && len(f.RequiresTools) > 0 {
			switch {
			case !probed:
				c.Available, c.Blocker = false, "tool probe (tools/list) failed, so tool presence is unknown"
			default:
				for _, need := range f.RequiresTools {
					if !present[need] {
						c.Available = false
						c.Blocker = fmt.Sprintf("Studio Pro does not expose the %q tool in this session", need)
						break
					}
				}
			}
		}
		out = append(out, c)
	}
	return out
}

// capability looks up a capability by key, resolved for the connected session.
func (b *Backend) capability(key string) (Capability, bool) {
	for _, c := range b.capabilities().Features {
		if c.Key == key {
			return c, true
		}
	}
	return Capability{}, false
}

// canAuthor reports whether the backend may author the given capability against the
// connected server. The single gate the create paths consult — same table the agent
// report reads, so report and behavior cannot disagree. Unknown key → false (a gated
// method must have a table entry).
func (b *Backend) canAuthor(key string) bool {
	c, ok := b.capability(key)
	return ok && c.Available
}

// notAuthorable builds the rejection for a blocked capability, sourcing the reason
// from the table (the message is single-source too, not a hardcoded string). A
// session-specific Blocker wins over the table note, because "this Studio Pro
// session does not expose the tool" is more actionable than the generic limit.
func (b *Backend) notAuthorable(kind, name, key string) error {
	note := "not supported by this Studio Pro version over MCP"
	if c, ok := b.capability(key); ok {
		switch {
		case c.Blocker != "":
			note = c.Blocker
		case c.Note != "":
			note = c.Note
		}
	}
	return fmt.Errorf("%s %q is not authorable via the MCP backend — %s; create it against a local .mpr or in Studio Pro", kind, name, note)
}

// errCreatePathUnbuilt guards the (today unreachable) branch where the table marks a
// doc type authorable but its create path has not been implemented. Set
// `available: true` for a doc type only once both PED permits it AND that path exists.
func errCreatePathUnbuilt(kind, name string) error {
	return fmt.Errorf("%s %q: the capability table marks this authorable, but the MCP backend's create path for it is not implemented — build the path before flipping the table", kind, name)
}

// capabilities builds the effective capability set: the table resolved against
// the project's Mendix version, narrowed by the live tool probe, plus live
// identity.
//
// Cached for the session: every authoring gate calls this, and the probe is a
// network round-trip. tools.listChanged means the surface *can* move mid-session,
// but re-probing per gate would cost a round-trip on every write for a change
// mxcli has no way to act on mid-statement.
func (b *Backend) capabilities() Capabilities {
	if b.capsCache != nil {
		return *b.capsCache
	}
	caps := Capabilities{
		ServerName:    b.server.Name,
		ServerVersion: b.server.Version,
	}
	var pv *types.ProjectVersion
	if b.reader != nil {
		pv = b.ProjectVersion()
		if pv != nil {
			caps.ProjectVersion = pv.String()
		}
	}
	if b.client != nil {
		if tools, err := b.client.ListTools(); err == nil {
			caps.ToolsProbed = true
			for _, t := range tools {
				if strings.HasPrefix(t, federatedToolPrefix) {
					caps.FederatedTools = append(caps.FederatedTools, t)
					continue
				}
				caps.Tools = append(caps.Tools, t)
			}
			sort.Strings(caps.Tools)
			sort.Strings(caps.FederatedTools)
		}
	}
	// Only Studio Pro's own tools gate capability; federated ones are third-party
	// and mxcli does not control their contract (ADR-0006 Revision).
	caps.Features = resolveCapabilities(pv, caps.Tools, caps.ToolsProbed)
	// Memoize only a connected session. Caching a pre-Connect call would pin an
	// empty tool surface for the rest of the run, blocking every gated feature.
	if b.client != nil {
		b.capsCache = &caps
	}
	return caps
}

// CapabilityReport renders a human-readable summary of what the MCP backend can
// author against the connected Studio Pro server — so an agent can check, before
// generating MDL, which operations are possible against this version. It is
// generated entirely from (*Backend).capabilities (no hardcoded lists).
func (b *Backend) CapabilityReport() string {
	caps := b.capabilities()
	var sb strings.Builder
	sb.WriteString("MCP backend capabilities\n")
	sb.WriteString("========================\n")
	fmt.Fprintf(&sb, "Studio Pro MCP server : %s %s\n", orUnknown(caps.ServerName), orUnknown(caps.ServerVersion))
	fmt.Fprintf(&sb, "Project Mendix version: %s\n", orUnknown(caps.ProjectVersion))
	drop := "unavailable — needs Studio Pro 11.16+ (ped_delete_document)"
	if slices.Contains(caps.Tools, "ped_delete_document") {
		drop = "ped_delete_document"
	}
	fmt.Fprintf(&sb, "DROP of documents     : %s\n", drop)
	check := "unavailable — needs Studio Pro 11.15+ (ped_check_errors filters)"
	if b.client != nil && b.client.SupportsToolArg("ped_check_errors", "filters") {
		check = "ped_check_errors, project-wide"
	}
	fmt.Fprintf(&sb, "--mcp-check           : %s\n", check)
	run := "unavailable — needs Studio Pro 11.16+ (execute_handler mendix_app)"
	if slices.Contains(caps.Tools, "execute_handler") {
		run = "execute_handler mendix_app"
	}
	fmt.Fprintf(&sb, "--mcp-run             : %s\n\n", run)

	sb.WriteString("Authorable over MCP:\n")
	for _, c := range caps.Features {
		if c.Available {
			fmt.Fprintf(&sb, "  ✓ %s — %s\n", c.Feature, c.Note)
		}
	}
	sb.WriteString("\nNot authorable:\n")
	for _, c := range caps.Features {
		if !c.Available {
			reason := c.Note
			if c.Blocker != "" {
				reason = c.Blocker
			}
			fmt.Fprintf(&sb, "  ✗ %s — %s\n", c.Feature, reason)
		}
	}
	sb.WriteString("\nReads (SHOW / DESCRIBE of any document type): always available from the local .mpr.\n")

	if !caps.ToolsProbed {
		sb.WriteString("\n⚠ tools/list did not answer, so tool presence is unknown; tool-dependent\n" +
			"  features are reported unavailable rather than assumed present.\n")
	} else {
		fmt.Fprintf(&sb, "\nStudio Pro tools present (%d): %s\n", len(caps.Tools), strings.Join(caps.Tools, ", "))
	}
	if len(caps.FederatedTools) > 0 {
		fmt.Fprintf(&sb, "\nFederated tools (%d), proxied by Studio Pro from MCP servers you connected to it.\n"+
			"mxcli reports these but never relies on them — their contract is not mxcli's to guarantee:\n  %s\n",
			len(caps.FederatedTools), strings.Join(caps.FederatedTools, ", "))
	}
	sb.WriteString("\nThis report describes THIS session. Tool presence varies with your Studio Pro\n" +
		"preferences and connected MCP servers, not only with versions — quote it in bug\n" +
		"reports rather than a version number alone.\n")
	sb.WriteString("\nDetail & per-version onboarding: docs/03-development/PED_MCP_CAPABILITIES.md\n")
	return sb.String()
}

func orUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

// projectVersionAtLeast reports whether the project's Mendix version is at least
// want ("11.12"). A nil project version (no local reader) reports false, so a
// version-gated feature stays off rather than being assumed available.
//
// This replaces the previous serverVersionAtLeast gate, which was dead code: it
// compared MCP serverInfo.version, frozen at 1.0.0 across 11.11/11.12/11.13, so
// no `available_since` above the baseline could ever resolve true (ADR-0006
// Revision).
func projectVersionAtLeast(pv *types.ProjectVersion, want string) bool {
	if pv == nil {
		return false
	}
	major, minor, ok := splitMajorMinor(want)
	if !ok {
		return false
	}
	return pv.IsAtLeast(major, minor)
}

// splitMajorMinor parses "11.12" / "11.12.0" into (11, 12). It reports ok=false
// for anything it cannot parse, so a malformed table entry blocks the feature
// instead of silently gating on zero (which would make it always available).
func splitMajorMinor(v string) (major, minor int, ok bool) {
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}
