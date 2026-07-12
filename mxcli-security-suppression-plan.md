# Accepted-Findings Suppression for Security Rules

## Why this exists

Across the Toll fleet, the Security category consistently scores lower than
every other category, even for the cleanest apps (`Toll_StarterApp`,
`TollUserFrequencyTracker`). Calibration data confirms this isn't a
normalization artifact: Security's rate spread across 10 projects is only
**3.0x** (0.022 - 0.065), versus Architecture's 88.5x or Naming's 13.5x.
Every project in the fleet has a broadly similar density of Security
findings — this is a fleet-wide pattern, not a small-number-of-bad-apps
problem.

This matches what OLC/Toll already knows from prior static-analysis work:
the recurring findings have been reviewed and consciously accepted as low
priority for the most part, not overlooked.

**The wrong fix** is to raise Security's scoring baseline to make the score
look better. That's a blunt, permanent change to how strict the *entire
category* is, for *every* project, forever. If a genuinely new and
dangerous security issue appears later — e.g. someone removes an access
rule on a real entity — it would be diluted by the same generous baseline
introduced to account for today's already-reviewed noise. The score would
stop distinguishing "known and accepted" from "new and unreviewed."

**The right fix** is to suppress the *specific findings* that have been
reviewed and accepted (a specific rule, on a specific entity/page/module),
while leaving the scoring formula itself strict. New findings — a rule
that's never fired before, or the same rule appearing somewhere not yet
reviewed — still count at full severity. This mirrors standard
vulnerability-management practice: a risk-acceptance register for specific
findings, not a blanket policy change for an entire class of checks.

## What already exists in mxcli (no new subsystem needed)

Investigation of the codebase found that most of the plumbing for this
already exists — it just isn't used by the Security rules yet:

- **`linter.Configurable` interface** (`mdl/linter/linter.go`): any rule that
  implements `Configure(options map[string]any)` can receive per-rule
  options from a config file.
- **`RuleConfig` / `Run()`** (`mdl/linter/linter.go`): the linter already
  calls `Configure()` on any rule implementing `Configurable`, using
  options loaded from a config file, before calling `Check()`.
- **`Config` / `LoadConfig()` / `ApplyConfig()`** (`mdl/linter/config.go`):
  already parses a YAML config file (`.claude/lint-config.yaml`,
  `lint-config.yaml`, or `.lint-config.yaml`) with per-rule
  `enabled` / `severity` / `options` blocks.
- **Starlark rules already use this** via a built-in `get_option(key,
  default)` function, wired through `StarlarkRule.Configure()`.

**The gap:** the four Go-native Security rules (`SEC001` /
`NoEntityAccessRulesRule`, `SEC002` / `WeakPasswordPolicyRule`, `SEC003` /
`DemoUsersActiveRule`, and `PageNavigationSecurityRule` in
`mdl/linter/rules/security.go` and `page_navigation_security.go`) do not
implement `Configurable` at all. There is currently no way to suppress a
specific accepted finding for any of them — only a blunt fleet-wide
`enabled`/`severity` override, or excluding an entire module.

## The fix: a shared, reusable `AcceptedFindings` helper

Rather than duplicating suppression logic across four rule files, add one
small struct the rules can embed to gain `Configurable` support via Go's
method promotion.

**New code** (add to `mdl/linter/rules/security.go`, or a new shared file):

```go
// AcceptedFindings gives a rule Configure() support for an accepted-findings
// list, read from lint-config.yaml under that rule's `options.accepted` key.
// Embed this in a rule struct to get Configurable for free.
type AcceptedFindings struct {
	accepted map[string]bool
}

// Configure implements linter.Configurable.
func (a *AcceptedFindings) Configure(options map[string]any) {
	a.accepted = make(map[string]bool)
	raw, ok := options["accepted"]
	if !ok {
		return
	}
	list, ok := raw.([]any)
	if !ok {
		return
	}
	for _, item := range list {
		if s, ok := item.(string); ok {
			a.accepted[s] = true
		}
	}
}

// IsAccepted reports whether a qualified name has been marked as reviewed
// and accepted for this rule.
func (a *AcceptedFindings) IsAccepted(qualifiedName string) bool {
	return a.accepted[qualifiedName]
}
```

### Wire it into each of the four Security rule structs

Change each struct definition to embed `AcceptedFindings`:

```go
type NoEntityAccessRulesRule struct{ AcceptedFindings }
type WeakPasswordPolicyRule struct{ AcceptedFindings }
type DemoUsersActiveRule struct{ AcceptedFindings }
type PageNavigationSecurityRule struct{ AcceptedFindings }
```

The existing `New*Rule()` constructors need no changes — the embedded
zero value works fine until `Configure()` is actually called by the
linter.

### Add one skip-check per rule, before each violation is appended

Example for `SEC001` (`NoEntityAccessRulesRule.Check`):

```go
for e := range ctx.Entities() {
	if e.EntityType != "Persistent" || e.IsExternal || e.AccessRuleCount > 0 {
		continue
	}
	if r.IsAccepted(e.QualifiedName) {
		continue
	}
	violations = append(violations, linter.Violation{ /* ...unchanged... */ })
}
```

Apply the same one-line pattern (`if r.IsAccepted(...) { continue }`) to
the other three rules, matching whatever qualified-name field each one
already uses to build its `Violation.Location`.

## The config file

Add or update `.claude/lint-config.yaml` in a project to record what's
been reviewed and accepted:

```yaml
rules:
  SEC001:
    options:
      accepted:
        - "Administration.User"
        - "System.AutoCommitEntry"
  SEC002:
    options:
      accepted: []
```

Each entry under `accepted` is the qualified name (`Module.EntityName`, or
whatever identifier that specific rule keys its violations on) that has
been reviewed and is not expected to be fixed. Anything not on this list —
including new findings on modules/entities not yet reviewed — is still
flagged at full severity.

## Verify before rolling this out: a related existing behavior to check

`Linter.Run()` currently does this **unconditionally** for any rule that
has *any* config entry at all:

```go
if config, ok := l.configs[rule.ID()]; ok {
	for i := range violations {
		violations[i].Severity = config.Severity
	}
}
```

This means: the moment a rule gets a `.claude/lint-config.yaml` entry for
any reason — even one that only sets `options.accepted` and never touches
`severity` — every surviving violation's severity gets overwritten by
`config.Severity`. If `ApplyConfig()` does not explicitly default
`config.Severity` to the rule's own `DefaultSeverity()` when the YAML
omits a `severity:` field, adding an `options:` block (with no
`severity:` line) could silently zero out the severity of every violation
that rule still produces.

**Check this before implementing the above:**

```bash
sed -n '52,75p' mdl/linter/config.go
```

- **If `ApplyConfig()` already defaults an empty `severity` to the rule's
  real default severity** — safe to proceed exactly as described above.
- **If it does not** — fix that default first (in `ApplyConfig()`, when
  the YAML's `severity` field is empty/unset, set
  `config.Severity = rule.DefaultSeverity()` before storing it), as a
  small, separate bug fix ahead of adding suppression.

## Testing after implementation

1. Rebuild: `go build -o bin/mxcli ./cmd/mxcli` (or `make build` if any
   Starlark/grammar files also changed).
2. Pick one project with known Security findings and add an
   `accepted:` entry for one specific finding you can see in its current
   report.
3. Re-run `report` on that project and confirm:
   - The specific accepted finding no longer appears.
   - Any *other* Security findings on that same project still appear,
     with their original severity (not silently downgraded — this
     directly checks the risk flagged above).
4. Re-run the full fleet calibration (`calibrate.sh`) once a handful of
   real, reviewed findings have been added across projects, and confirm
   Security's rate spread widens — i.e. projects that have gone through
   review now score meaningfully differently from ones that haven't,
   rather than all clustering in the same narrow band as before.
    - ./calibrate.sh ./bin/mxcli "/mnt/c/Mendix/Toll/MAIN" calibration.json
