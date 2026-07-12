#!/usr/bin/env python3
"""
compute_baseline.py -- derive a per-category normalizationBaseline map from
a calibration.json produced by calibrate.sh, and sanity-check the resulting
scores against every project actually in that calibration run.

Formula being calibrated (mdl/linter/report.go, buildCategoryScore):

    penalty(raw)        = errors*W_e + warnings*W_w + infos*W_i
    penalty(normalized) = penalty(raw) / elementsChecked * normalizationBaseline
    Score                = max(0, 100 - penalty(normalized))

This script picks normalizationBaseline per category so that a chosen
percentile of that category's *rate* (penalty(raw)/elementsChecked) across
the calibration set maps to a chosen target score. See the "How to
recalibrate" section of the accompanying markdown doc for how to choose
ANCHOR_PERCENTILE and TARGET_SCORE deliberately, rather than by feel.

Usage:
    python3 compute_baseline.py <calibration.json> [anchor_percentile] [target_score]

Defaults to the values this project settled on: p90 -> 60.
"""
import json
import sys
import statistics
from collections import defaultdict


def percentile(values, p):
    values = sorted(values)
    k = (len(values) - 1) * (p / 100)
    f = int(k)
    c = min(f + 1, len(values) - 1)
    if f == c:
        return values[f]
    return values[f] + (values[c] - values[f]) * (k - f)


def main():
    if len(sys.argv) < 2:
        print("Usage: compute_baseline.py <calibration.json> [anchor_percentile] [target_score]")
        sys.exit(1)

    path = sys.argv[1]
    anchor_percentile = float(sys.argv[2]) if len(sys.argv) > 2 else 90.0
    target_score = float(sys.argv[3]) if len(sys.argv) > 3 else 60.0
    target_penalty = 100.0 - target_score

    with open(path) as f:
        data = json.load(f)
    projects = data["projects"] if isinstance(data, dict) and "projects" in data else data

    rates_by_cat = defaultdict(list)
    for proj in projects:
        for cat in proj.get("categories", []):
            if cat.get("elementsChecked", 0) > 0:
                rates_by_cat[cat["name"]].append(cat["rate"])

    cats = sorted(rates_by_cat.keys())

    print(f"Calibrating against {len(projects)} project(s).")
    print(f"Anchor: p{anchor_percentile:g} rate -> score {target_score:g} "
          f"(target penalty {target_penalty:g})\n")

    # Warn on thin data -- a percentile computed from very few projects is
    # not a reliable anchor and should be treated with extra suspicion.
    MIN_RELIABLE_N = 5

    baselines = {}
    print(f"{'Category':<14}{'n':>4}{'median':>10}{'anchor':>10}{'baseline':>14}")
    for cat in cats:
        rates = rates_by_cat[cat]
        n = len(rates)
        med = statistics.median(rates)
        anchor_rate = percentile(rates, anchor_percentile)
        baseline = (target_penalty / anchor_rate) if anchor_rate > 0 else None
        baselines[cat] = baseline
        warn = "  <-- few data points, treat with caution" if n < MIN_RELIABLE_N else ""
        baseline_str = f"{baseline:.2f}" if baseline is not None else "n/a (anchor rate = 0)"
        print(f"{cat:<14}{n:>4}{med:>10.4f}{anchor_rate:>10.4f}{baseline_str:>14}{warn}")

    # Categories where the anchor rate was exactly 0 (nothing ever fires at
    # that percentile) get no computable baseline. Fall back to the median
    # of all *other* computed baselines, since leaving normalizationBaseline
    # undefined for a category would silently skip normalization for it.
    computed = [b for b in baselines.values() if b is not None]
    fallback = statistics.median(computed) if computed else 50.0
    for cat in cats:
        if baselines[cat] is None:
            baselines[cat] = round(fallback, 2)
            print(f"\n'{cat}' had no violations at p{anchor_percentile:g} -- "
                  f"using fallback baseline {fallback:.2f} (median of other categories).")
        else:
            baselines[cat] = round(baselines[cat], 2)

    print("\nResulting scores per project (sanity check):")
    header = f"{'Project':<40}"
    for cat in cats:
        header += f"{cat[:8]:>10}"
    print(header)
    for proj in projects:
        rates_lookup = {c["name"]: c["rate"] for c in proj.get("categories", [])}
        row = f"{proj['project']:<40}"
        for cat in cats:
            rate = rates_lookup.get(cat, 0)
            score = max(0.0, 100.0 - rate * baselines[cat])
            row += f"{score:>10.1f}"
        print(row)

    print("\nvar normalizationBaseline = map[string]float64{")
    for cat in cats:
        print(f'\t"{cat}": {baselines[cat]},')
    print("}")


if __name__ == "__main__":
    main()
