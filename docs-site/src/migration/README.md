# Migration Guide

mxcli and MDL enable AI-assisted migration of existing applications to the Mendix platform. An AI coding agent (Claude Code, GitHub Copilot, OpenCode, Cursor, Windsurf) investigates the source application, maps its elements to Mendix concepts, generates MDL scripts, and validates the result -- all from the command line.

This part covers the five-phase migration workflow and the skills that support each phase.

## Why mxcli for Migration?

Traditional migrations require deep Mendix expertise and manual work in Studio Pro. With mxcli:

- **AI agents do the heavy lifting** -- the agent reads source code, proposes a transformation plan, and generates MDL scripts
- **Skills provide guardrails** -- platform-specific migration skills guide the agent with correct mappings, naming conventions, and patterns
- **Validation is automated** -- `mxcli check`, `lint`, `docker check`, and `test` catch errors before anyone opens Studio Pro
- **The process is repeatable** -- MDL scripts can be version-controlled, reviewed, and re-run

## The Five Phases

```
  Phase 1           Phase 2           Phase 3           Phase 4         Phase 5
 ┌──────────┐    ┌───────────┐   ┌──────────────┐   ┌──────────┐   ┌──────────┐
 │  Assess  │--> │  Propose  │-->│   Generate   │-->│   Test   │-->│  Finish  │
 │  Source  │    │ Transform │   │  Mendix App  │   │  & Lint  │   │ in Studio│
 └──────────┘    └───────────┘   └──────────────┘   └──────────┘   │   Pro    │
                                                                   └──────────┘
```

| Phase | What Happens | Key Skills |
|-------|-------------|------------|
| [Assessment](assessment.md) | Investigate source application, produce inventory | `assess-migration`, platform-specific skills |
| [Transformation](transformation.md) | Map source elements to Mendix, plan module structure | Assessment output as input |
| [Generation](generation.md) | Generate domain model, microflows, pages, security | `generate-domain-model`, `write-microflows`, `create-page`, `manage-security` |
| [Data Migration](data-migration.md) | Import data from source databases | `demo-data`, `database-connections` |
| [Validation and Handoff](validation.md) | Test, lint, and hand off to Studio Pro | `check-syntax`, `report-quality`, `assess-quality` |
