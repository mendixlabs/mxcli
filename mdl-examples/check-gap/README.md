# check-gap: broken-script fixtures for the check-vs-Mendix parity harness

Each `fixtures/fNN_*.mdl` is a small script that `mxcli exec --no-check` writes
happily but a Mendix checker rejects (or, for `f10`, accepts: a neutral control).
`fixtures/00_base.mdl` creates the module the fixtures build on and is prepended
to each. See `docs/11-proposals/PROPOSAL_check_diagnostics_catalog.md`
(harvest-against-the-oracle loop).

`run-parity.sh` writes every fixture into a copy of a clean project and prints
the error codes reported by `mxcli check -p` (pre-write), MxBuild `mx check`,
and optionally Mendix CLI 2 `mendix check` (a proprietary binary that is never
committed; pass its path). Gated: needs mxbuild, so it is not part of `make test`.

    mxcli setup mxbuild --version 11.16.1
    mxcli new Spike --version 11.16.1 --skip-build --skip-init --theme none \
        --layout none --output-dir /tmp/clean
    mdl-examples/check-gap/run-parity.sh --project /tmp/clean/Spike.mpr \
        --mx ~/.mxcli/mxbuild/11.16.1/modeler/mx [--mendix /path/to/mendix]

A fixture that `mxcli check -p` passes while `mx check` reports a code is a
backlog item. Codes can differ between checkers for the same finding (CE1613 in
mx is CE7710 in `mendix check`), so compare messages when a code differs.

`run-parity.sh` compares what each checker reports for one defect at a time. To see what a
checker HIDES when two defects share a document (MxBuild reports in stages; Mendix CLI 2 does
not), use `scripts/maskmatrix`:

    make mask-matrix ARGS="--project /tmp/clean/Spike.mpr \
        --mx ~/.mxcli/mxbuild/11.16.1/modeler/mx [--mendix /path/to/mendix]"

It writes every defect and every ordered pair into a copy of the project, checks once, and
prints who hides whom, the stages that gives, and what an unresolved reference hides.
