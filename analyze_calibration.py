#!/usr/bin/env python3
"""
Reads calibration.json (an array of {"project": ..., "categories": [...]})
produced by calibrate.sh, and reports percentile distributions of the raw
penalty/elementsChecked rate per lint category -- so you can pick a
toleratedDefectRate grounded in your actual fleet of Mendix apps rather
than guessing at a constant.

Note on units: the "rate" field here is penalty/elementsChecked, where
penalty = errors*10 + warnings*3 + infos*1. Since an all-errors category
would have penalty == elementsChecked*10, "rate" is 10x the equivalent
toleratedDefectRate used in the score formula. This script divides by 10
in the final suggestion so you can paste the numbers directly into the
Go constant.
"""
import json
import sys
from collections import defaultdict


def percentile(data, p):
    if not data:
        return None
    data = sorted(data)
    k = (len(data) - 1) * (p / 100)
    f = int(k)
    c = min(f + 1, len(data) - 1)
    if f == c:
        return data[f]
    return data[f] + (data[c] - data[f]) * (k - f)


def main():
    if len(sys.argv) != 2:
        print("Usage: analyze_calibration.py <calibration.json>")
        sys.exit(1)

    with open(sys.argv[1]) as f:
        data = json.load(f)

    # calibrate.sh now writes {"summary": ..., "projectCount": ..., "projects": [...]}
    # instead of a bare array. Support both shapes so an older calibration.json
    # (from before this change) still works.
    if isinstance(data, dict) and "projects" in data:
        projects = data["projects"]
    else:
        projects = data

    rates_by_category = defaultdict(list)
    for proj in projects:
        for cat in proj["categories"]:
            if cat["elementsChecked"] > 0:  # skip categories with nothing to check
                rates_by_category[cat["name"]].append(cat["rate"])

    print(f"Calibration data from {len(projects)} project(s)\n")
    print(f"{'Category':<14} {'n':>4} {'p50':>8} {'p75':>8} {'p90':>8} {'p95':>8} {'max':>8}")
    print("-" * 66)
    for cat, rates in sorted(rates_by_category.items()):
        p50 = percentile(rates, 50)
        p75 = percentile(rates, 75)
        p90 = percentile(rates, 90)
        p95 = percentile(rates, 95)
        mx = max(rates)
        print(f"{cat:<14} {len(rates):>4} {p50:>8.3f} {p75:>8.3f} {p90:>8.3f} {p95:>8.3f} {mx:>8.3f}")

    print()
    print("Suggested toleratedDefectRate per category (p75, converted to the")
    print("units the Go score formula expects -- i.e. rate/10):\n")
    print("var toleratedDefectRate = map[string]float64{")
    for cat, rates in sorted(rates_by_category.items()):
        suggested = percentile(rates, 75) / 10
        print(f'\t"{cat}": {suggested:.4f},')
    print("}")


if __name__ == "__main__":
    main()
