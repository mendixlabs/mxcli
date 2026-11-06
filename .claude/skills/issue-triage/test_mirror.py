#!/usr/bin/env python3
"""Scenario tests for mirror.py against a fake GitHub API (no network).

  python3 -I .claude/skills/issue-triage/test_mirror.py

Covers the 2026-10-22 incident, where a short listing of existing mirrors made
mirror.py re-create 40 of them, the duplicate-tracking bug it exposed, and the
`upstream-open` label on mirrors closed here as fixed while upstream is open.
"""
import contextlib, importlib.util, io, json, os, re, sys
sys.dont_write_bytecode = True  # loading mirror.py must not leave __pycache__ in the skill dir
UP, FORK = "up/repo", "me/repo"

def load(path):
    spec = importlib.util.spec_from_file_location("mirror_" + str(id(path)), path)
    mod = importlib.util.module_from_spec(spec); spec.loader.exec_module(mod)
    mod.time.sleep = lambda s: None
    return mod

def mirror(fork_no, up_no, state="open", updated="T", reason=None, labels=()):
    if state == "closed" and reason is None:
        reason = "completed"
    return {"number": fork_no, "state": state, "state_reason": reason,
            "labels": [{"name": n} for n in ("mirror", *labels)],
            "body": f"<!-- upstream: {UP}#{up_no} updated={updated} -->\nbody"}

def run(path, up_open, fork_mirrors, glitch=lambda page, calls: False, args=(), up_open_extra=()):
    """glitch(page, call_index) -> True makes that fork-listing page come back empty."""
    mod = load(path)
    calls = {"fork_pages": 0}
    writes = []
    up_issues = [{"number": n, "title": f"t{n}", "body": "b", "updated_at": "T", "created_at": "2026-01-01",
                  "comments": 0, "labels": [], "user": {"login": "u"}} for n in up_open]
    # the fork lists newest first, like GitHub
    fork_sorted = sorted(fork_mirrors, key=lambda m: -m["number"])
    def api(path, method="GET", data=None, ok_codes=()):
        if method != "GET":
            writes.append((method, path, data))
            return {}
        m = re.search(r"[?&]page=(\d+)", path); page = int(m.group(1)) if m else 1
        if path.startswith(f"/repos/{UP}/issues?"):
            return up_issues[(page-1)*100: page*100]
        if path.startswith(f"/repos/{FORK}/issues?") and "labels=mirror" in path:
            calls["fork_pages"] += 1
            if glitch(page, calls["fork_pages"]):
                return []
            return fork_sorted[(page-1)*100: page*100]
        if path.startswith(f"/repos/{FORK}/pulls"):
            return []
        if path.startswith(f"/repos/{UP}/issues/"):
            n = int(path.rsplit("/", 1)[1])
            return {"state": "open" if n in up_open or n in up_open_extra else "closed"}
        raise AssertionError(path)
    mod.api = api
    out = io.StringIO(); code = 0
    sys.argv = ["mirror.py", "--upstream", UP, "--fork", FORK, "--dry-run", *args]
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(io.StringIO()) as err:
        try:
            mod.main()
        except SystemExit as e:
            code = e.code if isinstance(e.code, int) else 1
            err.write(str(e.code))
    lines = out.getvalue().strip().splitlines()
    summary = json.loads(lines[-1]) if lines and code == 0 else None
    return code, summary, err.getvalue(), writes

def label_writes(writes, fork_no):
    """The upstream-open label calls made on one mirror."""
    return [(m, p) for m, p, d in writes if p.startswith(f"/repos/{FORK}/issues/{fork_no}/labels")
            and ("upstream-open" in p or "upstream-open" in json.dumps(d))]

def scenarios(path):
    res = {}
    ups = list(range(1, 251))
    fork = [mirror(1000 + n, n) for n in ups]            # 250 mirrors = 3 pages
    # A: page 3 empty once (transient) -> recovered, nothing created
    code, s, _, _ = run(path, ups, fork, glitch=lambda p, c: p == 3 and c == 3)
    res["A transient empty page recovers"] = code == 0 and s["created"] == []
    # B: page 3 empty persistently -> must refuse, never create duplicates
    code, s, _, _ = run(path, ups, fork, glitch=lambda p, c: p == 3)
    res["B short listing refused"] = code != 0
    # C: whole listing empty -> must refuse
    code, s, _, _ = run(path, ups, fork, glitch=lambda p, c: c == 1)
    res["C empty listing refused"] = code != 0
    # D: duplicate pair, newer copy closed -> refresh the open original
    code, s, _, _ = run(path, [7], [mirror(10, 7, "open", updated="OLD"), mirror(50, 7, "closed", updated="OLD")])
    res["D open original tracked"] = code == 0 and s["refreshed"] == [7]
    # E: fresh fork seeds normally, oldest first, capped
    code, s, _, _ = run(path, ups, [])
    res["E fresh seed works"] = code == 0 and s["created"] == list(range(1, 41))
    # F: new upstream issues beyond the newest mirrored are created normally
    code, s, _, _ = run(path, ups + [251, 252], fork)
    res["F new issues still mirrored"] = code == 0 and s["created"] == [251, 252]
    # G: backfill allowed explicitly
    code, s, _, _ = run(path, ups, fork, glitch=lambda p, c: p == 3, args=["--allow-backfill"])
    res["G --allow-backfill overrides"] = code == 0 and len(s["created"]) == 40
    # H: closed here as completed, still open upstream -> labelled and listed
    code, s, _, w = run(path, [5], [mirror(1005, 5, "closed")])
    res["H fixed here, open upstream: labelled"] = (code == 0 and (s or {}).get("fixed_here_open_upstream") == [5]
        and label_writes(w, 1005) == [("POST", f"/repos/{FORK}/issues/1005/labels")] and s["created"] == [])
    # I: already labelled -> listed, no second label call
    code, s, _, w = run(path, [5], [mirror(1005, 5, "closed", labels=["upstream-open"])])
    res["I already labelled: no repeat"] = code == 0 and (s or {}).get("fixed_here_open_upstream") == [5] and label_writes(w, 1005) == []
    # J: upstream closed it since -> label removed, not listed
    code, s, _, w = run(path, [6], [mirror(1006, 6), mirror(1005, 5, "closed", labels=["upstream-open"])])
    res["J closed upstream: label removed"] = (code == 0 and (s or {}).get("fixed_here_open_upstream") == []
        and label_writes(w, 1005) == [("DELETE", f"/repos/{FORK}/issues/1005/labels/upstream-open")])
    # K: closed here as not planned -> not a fix, not labelled
    code, s, _, w = run(path, [5], [mirror(1005, 5, "closed", reason="not_planned")])
    res["K not planned: left alone"] = code == 0 and (s or {}).get("fixed_here_open_upstream") == [] and label_writes(w, 1005) == []
    # L: an open mirror exists (the closed one is a duplicate) -> nothing to flag
    code, s, _, w = run(path, [5], [mirror(1005, 5), mirror(1050, 5, "closed", reason="duplicate")])
    res["L open mirror wins over closed duplicate"] = (code == 0 and (s or {}).get("fixed_here_open_upstream") == []
        and label_writes(w, 1005) == [] and label_writes(w, 1050) == [])
    # M: upstream issue missing from a short listing but still open -> label kept, still listed
    code, s, _, w = run(path, [6], [mirror(1006, 6), mirror(1005, 5, "closed", labels=["upstream-open"])], up_open_extra=[5])
    res["M short upstream listing keeps the label"] = (code == 0 and (s or {}).get("fixed_here_open_upstream") == [5]
        and label_writes(w, 1005) == [])
    # N: reopened here while still open upstream -> label removed, not listed
    code, s, _, w = run(path, [5], [mirror(1005, 5, "open", labels=["upstream-open"])])
    res["N reopened mirror: label removed"] = (code == 0 and (s or {}).get("fixed_here_open_upstream") == []
        and label_writes(w, 1005) == [("DELETE", f"/repos/{FORK}/issues/1005/labels/upstream-open")])
    return res

if __name__ == "__main__":
    here = os.path.dirname(os.path.abspath(__file__))
    results = scenarios(os.path.join(here, "mirror.py"))
    for k, v in results.items():
        print(("PASS " if v else "FAIL ") + k)
    sys.exit(0 if all(results.values()) else 1)
