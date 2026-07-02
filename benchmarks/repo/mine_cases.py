#!/usr/bin/env python3
"""Mine SWE-bench-style cases from esbuild's git history.

A case is a bug-fix commit that ships a regression test (test files and/or
snapshots) plus a small separable source fix. Each candidate is validated
mechanically in a scratch worktree:

  1. reset to the fix's PARENT commit
  2. overlay only the test/snapshot files from the fix commit
  3. the narrowed `go test` must FAIL       (the bug reproduces)
  4. additionally checkout the source fix — the test must PASS

Only validated cases are written to cases.json.

  python3 mine_cases.py <esbuild-clone> <scratch-worktree-dir> [max_cases]
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

CLONE = Path(sys.argv[1]).resolve()
WT = Path(sys.argv[2]).resolve()
MAX_CASES = int(sys.argv[3]) if len(sys.argv) > 3 else 12
OUT = Path(__file__).resolve().parent / "cases.json"

FIX_RE = re.compile(r"\bfix(es|ed)?\b|#\d{3,}", re.I)
SKIP_RE = re.compile(r"typo|comment|docs\b|changelog|readme|lint|gofmt|chore|ci\b|version", re.I)


def git(cwd, *args, timeout=600):
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True, timeout=timeout)


def is_overlay(path: str) -> bool:
    return path.endswith("_test.go") or "/snapshots/" in path or "/testdata/" in path


def candidates(n=800):
    log = git(CLONE, "log", f"-{n}", "--pretty=%H\x01%s", "--numstat").stdout
    cur, out = None, []
    for line in log.splitlines():
        if "\x01" in line:
            if cur:
                out.append(cur)
            sha, subject = line.split("\x01", 1)
            cur = {"sha": sha, "subject": subject, "overlay": [], "code": [], "codelines": 0}
            continue
        if cur is None or not line.strip():
            continue
        parts = line.split("\t")
        if len(parts) != 3:
            continue
        add, rm, path = parts
        n_ = (0 if add == "-" else int(add)) + (0 if rm == "-" else int(rm))
        if is_overlay(path):
            cur["overlay"].append(path)
        elif path.endswith(".go") and "vendor/" not in path:
            cur["code"].append(path)
            cur["codelines"] += n_
    if cur:
        out.append(cur)
    return [c for c in out
            if FIX_RE.search(c["subject"]) and not SKIP_RE.search(c["subject"])
            and c["overlay"] and 1 <= len(c["code"]) <= 3 and 0 < c["codelines"] <= 60]


def test_pkgs(overlay):
    pkgs = set()
    for f in overlay:
        d = Path(f).parent
        while d.name in ("snapshots", "testdata"):
            d = d.parent
        pkgs.add("./" + d.as_posix() + "/")
    return sorted(pkgs)


def run_tests(pkgs, timeout=420):
    r = subprocess.run(["go", "test", *pkgs], cwd=WT, capture_output=True, text=True, timeout=timeout)
    return r.returncode == 0


def reset_overlay(case):
    git(WT, "reset", "--hard", case["parent"])
    git(WT, "clean", "-fdx", "-e", ".gdbg")
    git(WT, "checkout", case["sha"], "--", *case["overlay"])


def validate(case) -> bool:
    parent = git(CLONE, "rev-parse", case["sha"] + "^").stdout.strip()
    if not parent:
        return False
    case["parent"] = parent
    case["pkgs"] = test_pkgs(case["overlay"])
    try:
        reset_overlay(case)
        if run_tests(case["pkgs"]):
            return False  # not red: overlaid test passes without the fix
        git(WT, "checkout", case["sha"], "--", *case["code"])
        return run_tests(case["pkgs"])  # green with the real fix
    except subprocess.TimeoutExpired:
        return False


def main():
    if not WT.exists():
        r = git(CLONE, "worktree", "add", "--detach", str(WT), "HEAD")
        if r.returncode != 0:
            raise SystemExit(f"worktree add failed: {r.stderr}")
    cands = candidates()
    print(f"{len(cands)} candidates; validating (target {MAX_CASES})…", flush=True)
    kept, per_pkg = [], defaultdict(int)
    for c in cands:
        pkg_key = c["code"][0].rsplit("/", 1)[0]
        if per_pkg[pkg_key] >= 4:
            continue
        ok = validate(c)
        print(f"  {c['sha'][:10]} {'OK ' if ok else 'no '} {c['subject'][:70]}", flush=True)
        if ok:
            kept.append({
                "sha": c["sha"], "parent": c["parent"], "bug": c["subject"],
                "overlay": c["overlay"], "code": c["code"],
                "codelines": c["codelines"], "pkgs": c["pkgs"],
            })
            per_pkg[pkg_key] += 1
        if len(kept) >= MAX_CASES:
            break
    OUT.write_text(json.dumps(kept, indent=2) + "\n")
    print(f"wrote {len(kept)} cases to {OUT}")


if __name__ == "__main__":
    main()
