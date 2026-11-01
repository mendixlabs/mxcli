# Linting and Reports

mxcli includes an extensible linting framework that checks Mendix projects for best practice violations, security issues, naming conventions, and architectural anti-patterns. The framework combines built-in Go rules with extensible Starlark rules.

## Overview

The linting system provides:

- **14 built-in Go rules** -- Fast, compiled rules for common issues
- **27 bundled Starlark rules** -- Extensible rules covering security, quality, architecture, design, and conventions
- **Custom rule support** -- Write your own rules in Starlark
- **Multiple output formats** -- Text, JSON, and SARIF for CI integration
- **Scored reports** -- Best practices report with category breakdowns

## Rule Categories

| Category | Prefix | Focus |
|----------|--------|-------|
| MDL | MDL001-MDL007 | Naming conventions, empty microflows, domain model size |
| Security | SEC001-SEC011 | Access rules, password policy, demo users, PII exposure |
| Convention | CONV001-CONV021 | Best practice conventions, error handling |
| Quality | QUAL001-QUAL004 | Complexity, documentation, long microflows |
| Architecture | ARCH001-ARCH003 | Cross-module data, entity business keys |
| Design | DESIGN001 | Entity attribute count |

## Quick Start

```bash
# Lint a project
mxcli lint -p app.mpr

# List available rules
mxcli lint -p app.mpr --list-rules

# JSON output for CI
mxcli lint -p app.mpr --format json

# Generate a scored report
mxcli report -p app.mpr
```

## Related Pages

- [Built-in Rules](builtin-rules.md) -- The 14 built-in Go rules
- [Starlark Rules](starlark-rules.md) -- The 27 bundled Starlark rules
- [Writing Custom Rules](custom-rules.md) -- How to write your own rules
- [mxcli lint](mxcli-lint.md) -- Command-line usage
- [mxcli report](mxcli-report.md) -- Best practices report
