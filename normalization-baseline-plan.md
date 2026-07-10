# Per-Category Normalization Baseline

## Why this exists

`buildCategoryScore` (in `mdl/linter/report.go`) currently normalizes every
category's penalty against a single flat constant:

```go
const normalizationBaseline = 50
```

Calibration data across 10 Toll projects showed this doesn't work: the
natural violation *rate* (`penalty / elementsChecked`) varies by up to
**88.5x** between categories (Architecture: 0.011-0.99, versus Performance:
0.00001-0.0005). A single shared constant can't be simultaneously right for
both — tuned so Architecture is discriminating, every low-rate category
(Quality, Security, Complexity, Performance, Design) becomes mathematically
incapable of scoring below the high-90s for *any* project, good or bad.
Tuned the other way, Architecture would clip straight to 0 for almost
every project.

**The fix:** replace the flat constant with a per-category map, each value
independently calibrated against that category's own real rate
distribution.

## Implementation steps

### 1. Change the constant to a map

In `mdl/linter/report.go`, replace:

```go
const normalizationBaseline = 50
```

with:

```go
// normalizationBaseline is calibrated per category against real project
// data (see compute_baseline.py and the "How to recalibrate" section
// below). Each value is chosen so a specific percentile of that
// category's real violation rate maps to a specific target score --
// NOT a single shared magic number, because categories' natural rates
// differ by orders of magnitude (see calibration data referenced in
// project history).
var normalizationBaseline = map[string]float64{
	"Architecture": 55.27,
	"Complexity":   36471.63,
	"Correctness":  2087.73,
	"Design":       4255.50,
	"Naming":       676.39,
	"Performance":  114384.76,
	"Quality":      1368.90,
	"Security":     822.88,
}

// defaultNormalizationBaseline is used for any category not present in the
// map above -- e.g. a brand-new category introduced by a new rule, before
// it's been through a calibration pass. Deliberately conservative (a
// middling value) rather than either extreme, so an uncalibrated category
// neither clips to 0 nor sits stuck near 100 by default.
const defaultNormalizationBaseline = 1000.0
```

(the specific values above come from a p90-rate-maps-to-score-60 calibration
against the 10-project fleet available at the time -- see "How to
recalibrate" if your project set or rule set has changed since)

### 2. Update `buildCategoryScore` to look up the per-category value

Find:

```go
if elementsChecked > 0 {
	penalty = penalty / float64(elementsChecked) * normalizationBaseline
}
```

Replace with:

```go
if elementsChecked > 0 {
	baseline, ok := normalizationBaseline[name]
	if !ok {
		baseline = defaultNormalizationBaseline
	}
	penalty = penalty / float64(elementsChecked) * baseline
}
```

(`name` is already the function's category-name parameter -- no signature
change needed)

### 3. Rebuild and verify

```bash
go build -o bin/mxcli ./cmd/mxcli && echo "BUILD OK" || echo "BUILD FAILED"
./bin/mxcli report -p "<path-to-a-known-project>.mpr" --format markdown -o report.md
```

Compare the Category Scores table against the "Resulting scores per
project" sanity-check table produced by `compute_baseline.py` (see below)
for that same project -- they should match.

Last bash line created a windows shareable executable.

## How to recalibrate

Recalibration should be redone whenever any of the following happens:

- **The set of projects changes meaningfully** -- new Toll apps added to
  the fleet, or old ones retired. The percentiles are only as good as the
  data they're computed from.
- **Rules are added, removed, or change what they check.** A new rule
  changes what a category's violations even represent; an excluded rule
  (e.g. via the accepted-findings suppression work) changes the rate
  distribution for that category, often significantly. Recalibrating
  *before* a wave of suppression changes roll out would just bake in the
  old, noisier rates -- recalibrate *after*, not before.
- **A denominator fix changes a category's element count** (as happened
  with Architecture's `pages_data` addition) -- this changes every
  project's rate for that category, so old baselines no longer mean what
  they used to.
  - **Important future potential improvement** -- Change the contract so every rule returns not just violations, but how many candidate items it evaluated — the rule that knows its own filter logic is the only thing that can never drift from it.

### Step 1 -- regenerate calibration data

```bash
./calibrate.sh ./bin/mxcli "<projects-dir>" calibration.json
```

This re-runs `mxcli init` (if needed), `mxcli report --raw`, and the
per-category summary across every project in `<projects-dir>`, same as
before.

### Step 2 -- run `compute_baseline.py`

```bash
python3 compute_baseline.py calibration.json <anchor_percentile> <target_score>
```

Example (the values used above):
```bash
python3 compute_baseline.py calibration.json 90 60
```

This prints, per category: how many projects had data, the median and
anchor-percentile rate, and the resulting baseline -- followed by a full
sanity-check table of every project's resulting score under that baseline,
and a ready-to-paste Go map.

### Step 3 -- choose the anchor deliberately, don't default blindly

The two numbers you pass (`anchor_percentile`, `target_score`) express a
policy decision, not a technical one. What they mean:

> "A project at the **Nth percentile** of this category's violation rate
> (across the calibration set) should score **S**."

Concretely:
- **Higher anchor percentile (e.g. p90)** = only the worst ~10% of
  projects get meaningfully penalized; the rest cluster in a comfortable
  range. Gentler, safer default -- good for a first pass or when the
  calibration set is small.
- **Lower anchor percentile (e.g. p50/median)** = half your fleet, by
  definition, sits at or below the anchor -- meaning half your projects
  will score at or below the target. This was tested during this
  project's own calibration and found **too aggressive**: several
  categories clipped straight to 0 for below-median projects, destroying
  the score's ability to distinguish "somewhat bad" from "catastrophic."
  Avoid anchoring below roughly p75 unless you've specifically checked
  for zero-clipping in the sanity-check table first.
- **Target score** controls how harshly the anchor point itself is
  penalized. A higher target score (e.g. 85) is lenient even at the
  anchor; a lower one (e.g. 60) is stricter.

**Always inspect the full sanity-check table `compute_baseline.py` prints
before adopting new values.** Look specifically for:
- Any score clipped to exactly `0.0` for a project you know isn't
  actually catastrophic in that category -- a sign the anchor is too
  aggressive for that category's distribution.
- Whether projects you already know are "good" or "bad" (from direct
  inspection, not just the score) land where you'd expect, relative to
  each other.

### Step 4 -- watch for thin data

`compute_baseline.py` flags any category with fewer than 5 projects
reporting data as low-confidence (`<-- few data points, treat with
caution`). A percentile computed from 2-3 projects is not a reliable
anchor -- it's especially sensitive to whichever project happens to sit
at the percentile boundary. Treat these categories' computed baselines as
provisional until more projects have reported into that category (e.g. a
brand-new rule that's only run against a handful of projects so far).

### Step 5 -- handle categories with an anchor rate of exactly zero

If a category has zero violations at the chosen percentile (e.g. a strict,
rarely-triggered rule where even p90 of projects report no findings),
there's no rate to divide by, and the script falls back to the **median of
all other categories' computed baselines** for that one category, with a
console warning. This is a reasonable default (keeps that category
roughly as sensitive as a "typical" category) but is not calibrated against
that category's own actual behavior -- if it starts producing real data
in a future run, recalibrate again rather than leaving it on the
fallback indefinitely.

### Step 6 -- update the map and rebuild

Paste the script's `var normalizationBaseline = map[string]float64{...}`
output directly into `mdl/linter/report.go`, replacing the previous map,
then rebuild (`go build -o bin/mxcli ./cmd/mxcli`) and spot-check a couple
of known projects' reports before relying on the new scores fleet-wide.

```bash
go build -o bin/mxcli ./cmd/mxcli && echo "BUILD OK" || echo "BUILD FAILED"
./bin/mxcli report -p "<path-to-a-known-project>.mpr" --format markdown -o report.md
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/mxcli-olc.exe ./cmd/mxcli
```
Last line creates a Windows executable, to share among others.