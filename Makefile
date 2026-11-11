# Makefile for ModelSDKGo
#
# Usage:
#   make build     - Build mxcli for current platform
#   make release   - Build mxcli for all platforms (macOS, Windows, Linux)
#   make test      - Run unit tests
#   make check-mdl - Check MDL syntax for all doctype example scripts
#   make test-integration - Run integration tests (requires mx/mxbuild)
#   make test-mdl  - Run MDL integration tests (requires Docker)
#   make lint      - Lint all code (Go + TypeScript)
#   make ci-local  - Run what CI's build-and-test job runs, before pushing
#   make lint-go   - Lint Go code (fmt + vet)
#   make lint-ts   - Lint TypeScript code (tsc --noEmit)
#   make grammar   - Regenerate ANTLR parser
#   make docs-site - Build documentation site (mdbook)
#   make docs-serve - Serve docs site locally with live reload
#   make sbom      - Generate CycloneDX SBOM (Go + TypeScript)
#   make sbom-report - Generate Markdown dependency report
#   make clean     - Remove build artifacts

BINARY_NAME = mxcli
BUILD_DIR = bin
CMD_PATH = ./cmd/mxcli

# Version info (can be overridden)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
# RELEASE_LDFLAGS additionally strips the symbol table (-s) and DWARF debug info
# (-w), which is ~25% of the binary (≈28MB on a ~112MB build) and unnecessary for
# distribution. Combined with -trimpath (GO_BUILD_FLAGS) for reproducible builds
# without local path leakage. The build-debug target keeps symbols for debugging.
RELEASE_LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
GO_BUILD_FLAGS = -trimpath

# Clean version for VS Code extension (must be valid semver: major.minor.patch)
VSCE_VERSION = $(shell echo "$(VERSION)" | sed 's/^v//; s/-.*//' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$$' || echo "0.0.0")

.PHONY: build build-debug size release clean test ci-local test-mdl check-mdl check-skill-mdl check-conformance conformance-shrink gen-migration-reference check-migration-reference check-skill-pack-js check-findings check-wiki-pages digest-status check-tunnel-deps check-test-timeouts check-widget-versions test-integration test-integration-executor test-integration-roundtrip test-integration-parity test-integration-upgrade test-integration-other grammar completions sync-skills sync-skill-packs sync-commands sync-mods sync-lint-rules sync-changelog sync-all docs documentation docs-site docs-serve vscode-ext vscode-install source-tree sbom sbom-report lint lint-go lint-ts fmt fmt-check vet skill-index check-skill-index bench

# Helper: copy file only if content differs (avoids mtime updates that invalidate go build cache)
# Usage: $(call copy-if-changed,src,dst)
define copy-if-changed
	@if [ ! -f $(2) ] || ! cmp -s $(1) $(2); then cp $(1) $(2); fi
endef

# Sync skills from .claude/skills/mendix to cmd/mxcli/skills for embedding
# Skills are directory-shaped (<name>/SKILL.md, Agent Skills standard), so this
# mirrors a tree rather than copying a flat list. --delete matters: a renamed or
# removed skill must not linger in the embed dir, or the binary keeps shipping it.
# Refresh the generated line-numbered section index at the top of each SKILL.md.
# Measured on BENCH-001 (2026-10-10): reading the skills was ~68% of a session's
# re-read cost, and the agent had no cheap way to read one section.
skill-index:
	@python3 -I scripts/skill-index.py

# Fails when a SKILL.md moved on without its index. A hand-kept table of
# contents is the drift this repo has already paid for twice.
check-skill-index:
	@python3 -I scripts/skill-index.py --check

sync-skills: skill-index
	@mkdir -p cmd/mxcli/skills
	@rsync -a --delete --exclude='.DS_Store' .claude/skills/mendix/ cmd/mxcli/skills/ 2>/dev/null \
		|| { rm -rf cmd/mxcli/skills && mkdir -p cmd/mxcli/skills && cp -R .claude/skills/mendix/. cmd/mxcli/skills/; }

# Sync skill packs from .claude/skills/packs to cmd/mxcli/skillpacks for embedding.
# Recursive, unlike sync-skills: a pack is a directory tree and flattening it
# would silently collide same-named files in different subdirectories.
sync-skill-packs:
	@mkdir -p cmd/mxcli/skillpacks
	@rsync -a --delete --exclude='.DS_Store' .claude/skills/packs/ cmd/mxcli/skillpacks/ 2>/dev/null \
		|| { rm -rf cmd/mxcli/skillpacks && mkdir -p cmd/mxcli/skillpacks && cp -R .claude/skills/packs/. cmd/mxcli/skillpacks/; }

# Sync Claude Code mods from mods/ to cmd/mxcli/mods for embedding. Only what a
# project runs is shipped: a mod's tests and the types the engine lays beside a
# loaded mod stay behind.
sync-mods:
	@mkdir -p cmd/mxcli/mods
	@rsync -a --delete --exclude='.DS_Store' --exclude='tests/' --exclude='*.test.ts' --exclude='*.test.tsx' \
		--exclude='.claude-plugin/types/' --exclude='tsconfig.json' mods/ cmd/mxcli/mods/ 2>/dev/null \
		|| { rm -rf cmd/mxcli/mods && mkdir -p cmd/mxcli/mods && cp -R mods/. cmd/mxcli/mods/ \
		  && find cmd/mxcli/mods \( -type d -name tests -o -path '*/.claude-plugin/types' \) -prune -exec rm -rf {} + \
		  && find cmd/mxcli/mods \( -name '*.test.ts' -o -name '*.test.tsx' -o -name tsconfig.json -o -name .DS_Store \) -delete; }

# Sync commands from .claude/commands/mendix to cmd/mxcli/commands for embedding
sync-commands:
	@mkdir -p cmd/mxcli/commands
	@changed=0; for f in .claude/commands/mendix/*.md; do \
		dst="cmd/mxcli/commands/$$(basename $$f)"; \
		if [ ! -f "$$dst" ] || ! cmp -s "$$f" "$$dst"; then \
			cp "$$f" "$$dst"; changed=$$((changed + 1)); \
		fi; \
	done; \
	if [ $$changed -gt 0 ]; then echo "Synced $$changed command file(s)"; fi

# Sync lint rules from .claude/lint-rules to cmd/mxcli/lint-rules for embedding
sync-lint-rules:
	@mkdir -p cmd/mxcli/lint-rules
	@changed=0; for f in .claude/lint-rules/*.star; do \
		dst="cmd/mxcli/lint-rules/$$(basename $$f)"; \
		if [ ! -f "$$dst" ] || ! cmp -s "$$f" "$$dst"; then \
			cp "$$f" "$$dst"; changed=$$((changed + 1)); \
		fi; \
	done; \
	if [ $$changed -gt 0 ]; then echo "Synced $$changed lint rule file(s)"; fi

# Sync VS Code extension (.vsix) for embedding — picks newest .vsix by mtime
sync-vsix:
	@src=$$(ls -t vscode-mdl/vscode-mdl-*.vsix 2>/dev/null | head -1); \
	if [ -n "$$src" ]; then \
		if [ ! -f cmd/mxcli/vscode-mdl.vsix ] || ! cmp -s "$$src" cmd/mxcli/vscode-mdl.vsix; then \
			cp "$$src" cmd/mxcli/vscode-mdl.vsix; \
			echo "Synced vscode-mdl.vsix ($$src)"; \
		fi; \
	elif [ ! -f cmd/mxcli/vscode-mdl.vsix ]; then \
		echo "Warning: No .vsix found. Creating empty placeholder."; \
		touch cmd/mxcli/vscode-mdl.vsix; \
	fi

# Sync changelog to cmd/mxcli for embedding
sync-changelog:
	$(call copy-if-changed,CHANGELOG.md,cmd/mxcli/changelog.md)

# Sync skills, commands, lint rules, and changelog
sync-all: sync-skills sync-skill-packs sync-mods sync-commands sync-lint-rules sync-vsix sync-changelog

# Generate LSP completion items from grammar (only rewrites file if content changed)
completions:
	@CGO_ENABLED=0 go run ./cmd/gen-completions -lexer mdl/grammar/MDLLexer.g4 -output cmd/mxcli/lsp_completions_gen.go.tmp
	@if [ ! -f cmd/mxcli/lsp_completions_gen.go ] || ! cmp -s cmd/mxcli/lsp_completions_gen.go.tmp cmd/mxcli/lsp_completions_gen.go; then \
		mv cmd/mxcli/lsp_completions_gen.go.tmp cmd/mxcli/lsp_completions_gen.go; \
		echo "Updated cmd/mxcli/lsp_completions_gen.go"; \
	else \
		rm cmd/mxcli/lsp_completions_gen.go.tmp; \
	fi

# Build for current platform (auto-syncs skills and commands)
build: grammar sync-all completions
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) $(RELEASE_LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)
	CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) -o $(BUILD_DIR)/source_tree ./cmd/source_tree
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME) ($$(du -h $(BUILD_DIR)/$(BINARY_NAME) | cut -f1)) $(BUILD_DIR)/source_tree"

# Build with debug tools (includes bson discover/compare/dump)
build-debug: sync-all completions
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -tags debug $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-debug $(CMD_PATH)
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)-debug (debug build with bson tools)"

# Report the built binary size (builds first if needed). Handy for catching size
# regressions before a release.
size: build
	@echo "$(BINARY_NAME): $$(du -h $(BUILD_DIR)/$(BINARY_NAME) | cut -f1) (stripped release build)"

# Build for all platforms (CGO_ENABLED=0 for cross-compilation)
release: clean grammar vscode-ext sync-all
	@mkdir -p $(BUILD_DIR)
	@echo "Building release binaries..."

	@echo "  -> Linux (amd64)"
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) $(RELEASE_LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)

	@echo "  -> Linux (arm64)"
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(GO_BUILD_FLAGS) $(RELEASE_LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)

	@echo "  -> macOS (amd64 - Intel)"
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(GO_BUILD_FLAGS) $(RELEASE_LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_PATH)

	@echo "  -> macOS (arm64 - Apple Silicon)"
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(GO_BUILD_FLAGS) $(RELEASE_LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)

	@echo "  -> Windows (amd64)"
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GO_BUILD_FLAGS) $(RELEASE_LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_PATH)

	@echo "  -> Windows (arm64)"
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(GO_BUILD_FLAGS) $(RELEASE_LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe $(CMD_PATH)

	@echo ""
	@echo "Release binaries:"
	@ls -lh $(BUILD_DIR)/

# Run tests
# `sync-all` is not optional here. The embed dirs (cmd/mxcli/skills, skillpacks,
# commands, lint-rules) are GENERATED from .claude/, and several tests read them
# back. `make build` has always synced first; `make test` did not, so a checkout
# where the skills layout had changed under a bare `go build` failed six tests in
# cmd/mxcli with an error that pointed at the go:embed directive rather than at
# the missing build step (mxcli-formula1 finding 68).
#
# -timeout is per test BINARY, and without one Go applies 10m. That is a
# coin-flip under `./...`: every package runs at once, so a package's wall time
# is set by the machine's load, not by its tests. mdl/linter passes in ~25s on
# its own and was measured at 552s and 872s inside `make test` — the second
# reported `panic: test timed out after 10m0s` as a FAIL in a package the PR had
# not touched (#1291). 30m is twice the slowest measurement. Override with
# `make test TEST_TIMEOUT=...`; scripts/check-test-timeouts.sh (make lint)
# refuses any go test here without a -timeout.
TEST_TIMEOUT ?= 30m

test: grammar sync-all
	CGO_ENABLED=0 go test -timeout $(TEST_TIMEOUT) ./...

# Check MDL syntax for all example scripts.
#
# Covers both doctype-tests/ (broad doctype demos) and bug-tests/ (per-PR
# regression fixtures). Skips files ending in .test.mdl — those are
# mxcli-test inputs, not mxcli-check inputs.
#
# Nine bug-test fixtures are pre-existing failures (varied causes: some
# demonstrate intentionally-broken syntax, others need project context).
# Tracked separately (issue #571); the SKIP list keeps CI green until each
# is triaged.
#
# Files ending in .fail.mdl are explicit negative tests — they MUST fail
# `mxcli check` (the script reproduces a symptom that a new validation
# rule rejects). The runner inverts the exit code for these: an unexpected
# pass is treated as a regression of the rule.
#
# A test file whose ANNOTATIONS are deliberately unusable is a negative test like
# any other and is named `.fail.test.mdl`; both fixtures of that kind exist to
# prove the runner reports an ERROR rather than a PASS.
#
# `.test.mdl` files are swept too. They used to be skipped because `check` could
# not parse one at all — a test block is a microflow body, not a top-level
# statement, so every test file reported errors about the grammar rather than
# about itself (mendixlabs/mxcli#1103). Now that `check` renders them, the sweep
# is what keeps that true.
#
# `check` runs here WITHOUT a project, so only CHECK-TIME rules can be tested
# this way. A guard living in the executor or a backend needs a model before it
# can decide anything, so its repro is valid MDL, `check` exits 0, and naming
# that file .fail.mdl reports "negative test unexpectedly passed" — a working
# rule made to look regressed (#891, #892). Keep those repros as plain .mdl and
# cover the guard with a unit test.
check-mdl: build
	@FAILED=0; \
	for f in mdl-examples/doctype-tests/*.mdl mdl-examples/bug-tests/*.mdl mdl-examples/deprecated-aliases/*.mdl; do \
		case "$$f" in \
			*/116-datagrid2-column-name-mismatch.mdl|\
			*/343-list-attribute-find-filter.mdl|\
			*/352-java-action-return-type-inference.mdl|\
			*/352-retrieve-compact-reverse-association.mdl|\
			*/360-import-mapping-result-type.mdl|\
			*/367-retrieve-sort-indirect-entity-ref.mdl|\
			*/369-rest-mapping-result-cardinality.mdl|\
			*/373-change-action-items-storage-list.mdl|\
			*/552-npe-reserved-words.mdl) \
				echo "SKIP: $$(basename "$$f") (pre-existing — tracked separately)"; \
				continue ;; \
		esac; \
		NAME=$$(basename "$$f"); \
		case "$$f" in *.fail.mdl|*.fail.test.mdl) \
			if ./$(BUILD_DIR)/$(BINARY_NAME) check "$$f" > /dev/null 2>&1; then \
				echo "FAIL (negative test unexpectedly passed): $$NAME"; \
				FAILED=1; \
			else \
				echo "PASS (negative test, expected error): $$NAME"; \
			fi; \
			continue ;; \
		esac; \
		if ./$(BUILD_DIR)/$(BINARY_NAME) check "$$f" > /dev/null 2>&1; then \
			echo "PASS: $$NAME"; \
		else \
			echo "FAIL: $$NAME"; \
			./$(BUILD_DIR)/$(BINARY_NAME) check "$$f" 2>&1 | grep -v "^WARNING"; \
			FAILED=1; \
		fi; \
	done; \
	exit $$FAILED

# Syntax-check the domain-model DDL statements embedded in the user skills and the
# docs site, so invalid MDL (e.g. enum `= 'x'`, association PARENT/CHILD, ALTER
# ENTITY `ADD (attr)` instead of `ADD ATTRIBUTE attr: type`) can't drift into docs.
check-skill-mdl: build
	@./scripts/check-skill-mdl.sh ./$(BUILD_DIR)/$(BINARY_NAME) .claude/skills/mendix
	@./scripts/check-skill-mdl.sh ./$(BUILD_DIR)/$(BINARY_NAME) .claude/skills/packs
	@# The syntax reference is the document people copy from, and it was never
	@# checked: four of its documented forms did not parse (`add (a, b)`,
	@# `drop (a)`, `rename X to Y`, `drop index (Col)`) — the exact drift class
	@# this script names in its own header. (sudoku findings #10)
	@./scripts/check-skill-mdl.sh ./$(BUILD_DIR)/$(BINARY_NAME) docs/01-project/MDL_QUICK_REFERENCE.md
	@# The script above checks fenced blocks in markdown. A pack also ships real
	@# .mdl files, which it does not see — and a pack whose own MDL is never
	@# checked is a pack that rots.
	@#
	@# Checked AFTER substitution, because that is the only form anyone runs. A
	@# pack's MDL may carry {{MODULE}} placeholders, which are not valid MDL and
	@# never reach a project un-substituted; checking the raw file would fail on
	@# every tokenised pack and tempt whoever hit it to drop the check instead.
	@for f in .claude/skills/packs/*/mdl/*.mdl; do \
		[ -e "$$f" ] || continue; \
		tmp=$$(mktemp /tmp/skillmdl-XXXXXX.mdl); \
		sed -e 's/{{MODULE_PATH}}/mymodule/g' -e 's/{{MODULE}}/MyModule/g' \
		    -e 's/{{NAMESPACE_PATH}}/acme/g' -e 's/{{NAMESPACE}}/acme/g' "$$f" > "$$tmp"; \
		./$(BUILD_DIR)/$(BINARY_NAME) check "$$tmp" >/dev/null || { echo "FAILED: $$f"; rm -f "$$tmp"; exit 1; }; \
		rm -f "$$tmp"; \
		echo "  ok $$f"; \
	done
	@./scripts/check-skill-mdl.sh ./$(BUILD_DIR)/$(BINARY_NAME) docs-site/src
	@# The syntax pass above splits blocks into statements and ignores semantic
	@# errors. Run each whole example through the full `mxcli check` as well:
	@# examples marked correct that check rejects shipped until a user ran
	@# the pack through check themselves (mendixlabs/mxcli#1357).
	@./scripts/check-skill-mdl-semantics.sh ./$(BUILD_DIR)/$(BINARY_NAME) .claude/skills/mendix .claude/skills/packs docs/01-project/MDL_QUICK_REFERENCE.md docs-site/src

# Canonical-form conformance gate (ako/mxcli#756, plan item 1.4 of
# PROPOSAL_mdl_beta_syntax_freeze.md). Parses every MDL block in `mxcli syntax`,
# the user-facing skills, docs-site, the quick reference and mdl-examples as
# mdl 1 (decision 1 on ako/mxcli#714), with a deprecated spelling (MDL-DEPRnnn),
# a construct the header refuses (MDL-V1-*) and a complete script without the
# `mdl 1;` header counted as failures, against the allowlist in mdl/conformance/allowlist.txt, which may only shrink: a count
# above its ceiling fails, and so does one below it. After fixing docs, run
# `make conformance-shrink`, which lowers the list and never raises it.
# mdl-examples/deprecated-aliases/ is exempt: it is the old-spelling corpus
# `fmt --upgrade` is proven on (see its README).
check-conformance:
	@go test ./mdl/conformance -count=1 -timeout 20m

# The "Language versions and migration" page's tables are generated from the
# language-change and deprecation registries (mdl/migration). check- is the CI
# staleness check: it writes nothing and fails when the page is behind.
gen-migration-reference:
	@go run ./cmd/gen-migration-reference

check-migration-reference:
	@go run ./cmd/gen-migration-reference -check

conformance-shrink:
	@MXCLI_CONFORMANCE_SHRINK=1 go test ./mdl/conformance -timeout 20m -run 'TestConformanceGate$$' -count=1

# Guard: the embedded tunnel (chisel) must stay out of the Windows/macOS builds.
# See docs/13-decisions/0009-tunnel-is-linux-only.md. Needs no build — it reads
# the dependency graph — so it is cheap to run before pushing.
# Validate the bug-finding shards: one JSON object per line, an area, and either
# the four structured fields or a raw row. DuckDB rejects a whole file on one bad
# line, so a typo in an appended finding takes out every query over that area.
check-findings:
	@scripts/check-findings.sh

# Unit-test the JavaScript a skill pack ships. Nothing else in this repo
# compiles a pack's widget — it is installed and built in the user's project —
# so logic that lives there has no gate at all unless it has one here.
#
# node's own test runner over .ts, no dependencies and no build step: Node 22
# strips the types. Keep pack test files to erasable syntax, and out of the
# widget's tsconfig `include` (they live in widget/test/, which it does not
# name), or the user's `npm run build` type-checks them without @types/node.
check-skill-pack-js:
	@found=0; \
	for f in .claude/skills/packs/*/widget/test/*.test.ts; do \
		[ -e "$$f" ] || continue; \
		found=1; \
		node --test "$$f" >/dev/null || { echo "FAILED: $$f"; node --test "$$f"; exit 1; }; \
		echo "  ok $$f"; \
	done; \
	[ $$found -eq 1 ] || echo "  (no skill-pack JS tests)"

# How far docs-wiki/bug-patterns/ has fallen behind the findings it digests.
# Advisory, always exits 0 — see the header in the script for why.
digest-status:
	@scripts/digest-status.sh

# The wiki's page list must describe the wiki, both directions. It drifted for
# three months before anyone noticed, because a table of contents has no failure
# mode of its own.
check-wiki-pages:
	@scripts/check-wiki-pages.sh

check-tunnel-deps:
	@./scripts/check-tunnel-deps.sh

# Every go test above must choose its own -timeout (#1291); see the script.
check-test-timeouts:
	@scripts/check-test-timeouts.sh

# Run integration tests (requires mx binary / mxbuild)
#
# The gate runs every doctype script through exec + mx check once PER ENGINE,
# and there is one engine since legacy was deleted. MXCLI_TEST_ENGINES is
# therefore left unset everywhere; it survives only so that a stale
# `MXCLI_TEST_ENGINES=legacy` is fatal rather than silently selecting nothing.
test-integration:
	CGO_ENABLED=0 go test -tags integration -count=1 -timeout 60m ./...

# The same integration tests split into suites that CI runs as parallel jobs
# (ako/mxcli#757): one step running them all spent ~22 of its 30 minutes once
# the upgrade property test landed, with mdl/roundtrip alone the critical path.
#
# Unlike test-integration, the suites run only the packages that HAVE
# integration-tagged tests. Every other package's tests are identical with and
# without the tag, and `make test` already runs them.
#
#   test-integration-executor   mdl/executor: doctype scripts through exec + mx check
#   test-integration-roundtrip  mdl/roundtrip: the round-trip laws, the upgrade controls
#   test-integration-upgrade    mdl/roundtrip: the upgrade property test alone;
#                               MXCLI_UPGRADE_SHARD=i/n runs one of n shards,
#                               MXCLI_UPGRADE_ALL=1 the full corpus (nightly)
#   test-integration-other      every other package with integration tests,
#                               found by build tag so a new one cannot be missed
#                               (hidden dirs are skipped: nested worktrees live there)
#
# `-skip` / `-run` split mdl/roundtrip by test name, so a new round-trip test
# lands in the roundtrip suite without editing this file.
INTEGRATION_PKGS = $(shell grep -rl --include='*_test.go' --exclude-dir='.?*' --exclude-dir=reference --exclude-dir=mx-test-projects \
	'^//go:build.*integration' . | xargs -n1 dirname | sed 's|^\./||; s|^|./|' | sort -u)
INTEGRATION_SPLIT = ./mdl/executor ./mdl/roundtrip
UPGRADE_PROPERTY = ^TestUpgradeExecutesToTheSameModel$$
# The TestApp splice-parity property (#839) runs describe → exec on every
# TestApp flow under both language versions (~7 min): its own CI suite. The
# TestSpliceRerun_ tests (#859) run there too, to keep the roundtrip suite
# under its time limit (#870).
SPLICE_PARITY = ^(TestTestAppFlowSpliceParity|TestSpliceRerun_.*|TestFlowVerdictAgreement_.*)$$
INTEGRATION_GO_TEST = CGO_ENABLED=0 go test -tags integration -count=1

test-integration-executor:
	$(INTEGRATION_GO_TEST) -timeout 40m ./mdl/executor/

test-integration-roundtrip:
	$(INTEGRATION_GO_TEST) -timeout 40m -skip '$(UPGRADE_PROPERTY)|$(SPLICE_PARITY)' ./mdl/roundtrip/

test-integration-parity:
	$(INTEGRATION_GO_TEST) -timeout 40m -run '$(SPLICE_PARITY)' ./mdl/roundtrip/

test-integration-upgrade:
	$(INTEGRATION_GO_TEST) -timeout 40m -run '$(UPGRADE_PROPERTY)' ./mdl/roundtrip/

test-integration-other:
	@pkgs="$(filter-out $(INTEGRATION_SPLIT),$(INTEGRATION_PKGS))"; \
	if [ -z "$$pkgs" ]; then echo "no integration packages outside the split suites"; exit 1; fi; \
	echo "integration packages: $$pkgs"; \
	$(INTEGRATION_GO_TEST) -timeout 40m $$pkgs

# Agent dev-loop benchmark: a fixed, scripted session of mxcli commands -- the
# ones an agent runs while building an app -- one process per command, at two
# project sizes, no model in the loop. Needs no mxbuild: it starts from the
# committed PedApp fixture.
#
#   make bench                                  # time this checkout's binary
#   make bench BASELINE=/path/to/other/mxcli    # A/B against another build
#   make bench BENCH_FLAGS='-sizes small -reps 3 -out record.json'
#
# With BASELINE the two binaries run interleaved on this machine and the table
# shows the ratio, which is the only figure comparable between two machines or
# two nights. The nightly `bench` job does exactly this against the previous
# nightly release and commits the record to the `benchmarks` branch.
# docs-site/src/internals/agent-benchmark.md explains how to read it.
BASELINE ?=
BENCH_FLAGS ?=
bench: build
	go run ./scripts/agentbench -mxcli $(BUILD_DIR)/$(BINARY_NAME) $(if $(BASELINE),-baseline $(BASELINE)) $(BENCH_FLAGS)

# Run MDL integration tests (requires Docker and a Mendix project)
# Usage: make test-mdl MPR=path/to/app.mpr
MPR ?= app.mpr
test-mdl: build
	./scripts/run-mdl-tests.sh "$(abspath $(MPR))" "$(abspath $(BUILD_DIR)/$(BINARY_NAME))"

# Cross-version widget-envelope drift gate (v0.12.0 Stream A / A3).
# Runs the v0.10 widget fixtures through exec + mx check on multiple Mendix
# versions and fails if the CE0463 set differs between them (envelope drift).
# Requires the matching mxbuilds (~/.mxcli/mxbuild/<ver>/) and reference
# projects with the fixtures' widgets installed (override the vars below).
MX_PROJECT_119 ?= ../ModelSDKGo/mx-test-projects/test5-app/test5.mpr
MX_PROJECT_1110 ?= ../ModelSDKGo/mx-test-projects/test6-app/test6.mpr
check-widget-versions: build
	@for fix in 03-page-examples 30-pluggable-widget-examples 31-pluggable-datagrid-gallery-v010-examples 32-pluggable-widget-object-lists-v010; do \
		echo "== $$fix =="; \
		./scripts/check-widget-versions.sh "mdl-examples/doctype-tests/$$fix.mdl" \
			"11.9.0:$(MX_PROJECT_119)" "11.10.0:$(MX_PROJECT_1110)" || exit 1; \
	done

# Lint all code (Go + TypeScript)
lint: lint-go lint-ts check-conformance check-test-timeouts

# What the quick-checks and build-and-test jobs in .github/workflows/push-test.yml
# run, in their order (the no-compile checks first, then vet before test --
# ako/mxcli#1887), so a push is proven against the gate it will meet rather than against
# `build test lint`, which covers less of it: check-mdl, check-findings,
# check-skill-index and the rest run only in CI unless someone remembers them.
# check-skill-mdl is a superset of the job's three skill/docs MDL steps.
#
# Left out: govulncheck (the job installs a pinned version and reads its
# database over the network) and the integration jobs, which need mxbuild --
# `make test-integration` runs those.
# A step added to the job belongs here too.
ci-local: fmt-check check-findings check-wiki-pages check-skill-index check-test-timeouts \
		build vet test check-tunnel-deps check-mdl check-skill-pack-js check-skill-mdl \
		check-conformance check-migration-reference
	@echo "ci-local passed (build-and-test, without govulncheck)"

# Lint Go code
#
# Depends on fmt-check, which VERIFIES, not on fmt, which rewrites. `go fmt ./...`
# edits in place and then exits 0, so this target could never fail on an
# unformatted file: main carried one indefinitely
# (mdl/executor/cmd_microflows_helpers.go, whose doc comment gofmt rewrote --
# `''` is the legacy godoc digraph for a closing curly quote, and the comment was
# about doubled quotes). Locally the rewrite also dirtied the tree on every
# `make build`, which is a trap for `git add -A`.
lint-go: fmt-check vet
	@echo "Go lint passed"

# Verify Go formatting without rewriting anything. `make fmt` is the fixer.
#
# Tracked files only: the ANTLR parser under mdl/grammar/parser is generated at
# build time and deliberately not committed. The empty-list guard is a positive
# control -- a check that inspected nothing must fail loudly rather than pass.
#
# The gofmt is the toolchain's, not the first one on PATH. go.mod's `toolchain`
# line makes `go` (and so `make fmt`) switch to that release, but a bare `gofmt`
# is whatever Go is installed, and gofmt's output changes between releases. With
# Go 1.24.7 installed and the toolchain at 1.27.3, the bare one failed main on a
# file CI accepts (mdl/backend/mfmutator/splice_handler_test.go) and passed a
# file CI then rejected (ako/mxcli#1759).
GOFMT ?= $(shell go env GOROOT)/bin/gofmt
fmt-check:
	@files=$$(git ls-files '*.go' | grep -v '/parser/'); \
	if [ -z "$$files" ]; then \
		echo "fmt-check: found no Go files to check -- the file list is wrong"; \
		exit 1; \
	fi; \
	unformatted=$$($(GOFMT) -l $$files); \
	if [ -n "$$unformatted" ]; then \
		echo "Not gofmt-clean (run 'make fmt'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

# Format Go code
fmt:
	go fmt ./...

# Vet Go code (filters out generated ANTLR parser warnings).
#
# Vetted once per build-tag set, and the `integration` pass is the one that earns
# its keep. Files behind `//go:build integration` are invisible to a bare
# `go vet ./...` AND to `make test`, so a rename can leave them uncompilable
# while the entire fast gate stays green. That is what happened when
# `resolveJDK21` became `resolveJDK(major)`: three PRs went red on a CI job that
# spends half an hour reaching a compile error the type-checker finds in seconds,
# and nothing anyone could run locally would have said so.
#
# Vet only type-checks here — it does not run a single integration test — so this
# costs a few seconds and needs no mx, no JDK and no Docker.
vet:
	@for tags in "" "integration"; do \
		out=$$(CGO_ENABLED=0 go vet -tags "$$tags" ./... 2>&1 | grep -v 'grammar/parser/' | grep -v 'mdl-grammar/parser/'); \
		if [ -n "$$out" ]; then echo "$$out"; fi; \
		if echo "$$out" | grep -q 'vet:'; then \
			echo "go vet failed (build tags: $${tags:-none})"; exit 1; \
		fi; \
	done

# Lint TypeScript code (VS Code extension)
lint-ts:
	cd vscode-mdl && bun install --silent && bun run lint
	@echo "TypeScript lint passed"

# Regenerate ANTLR parser from MDLLexer.g4 and MDLParser.g4
grammar:
	$(MAKE) -C mdl/grammar generate

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	go clean

# Build VS Code extension (.vsix) with build-time version info
vscode-ext:
	@echo "Building VS Code extension (version $(VSCE_VERSION))..."
	cd vscode-mdl && bun install && \
		cp package.json package.json.bak && \
		sed 's/"version": "[^"]*"/"version": "$(VSCE_VERSION)"/' package.json.bak > package.json && \
		bunx esbuild src/extension.ts --bundle --outfile=dist/extension.js \
			--external:vscode --format=cjs --platform=node \
			--define:__BUILD_TIME__="'$(BUILD_TIME)'" \
			--define:__GIT_COMMIT__="'$(VERSION)'" && \
		bunx @vscode/vsce package --no-dependencies; \
		status=$$?; mv package.json.bak package.json; exit $$status
	@echo "Built vscode-mdl/$$(ls vscode-mdl/*.vsix)"

# Install VS Code extension
vscode-install: vscode-ext
	code --install-extension vscode-mdl/vscode-mdl-*.vsix
	@echo "Extension installed. Reload VS Code to activate."

# Generate documentation from ANTLR4 grammar
docs: documentation
documentation:
	@echo "Generating MDL grammar documentation..."
	@mkdir -p docs/06-mdl-reference
	@CGO_ENABLED=0 go run ./cmd/grammardoc \
		-grammar mdl/grammar/MDLParser.g4 \
		-lexer mdl/grammar/MDLLexer.g4 \
		-output docs/06-mdl-reference/grammar-reference.md \
		-title "MDL Grammar Reference"
	@echo "Documentation generated at docs/06-mdl-reference/grammar-reference.md"

# Build documentation site with mdbook
docs-site:
	mdbook build docs-site

# Serve documentation site locally with live reload
docs-serve:
	mdbook serve docs-site

# Generate CycloneDX SBOM (Go + TypeScript dependencies)
sbom:
	@scripts/generate-sbom.sh

# Generate Markdown dependency report from SBOM
sbom-report: sbom
	@scripts/generate-sbom-report.sh

# Generate source tree overview
source-tree: build
	@$(BUILD_DIR)/source_tree --all > source_tree.txt
	@echo "Generated source_tree.txt"
