#!/usr/bin/env python3
"""Mine SWE-bench-style cases from kubernetes' git history.

Kubernetes lands PRs as merge commits (no squash), so the unit here is the
MERGE commit diffed against its first parent: that diff contains the whole
PR (fix + regression test) and the first parent is a clean pre-fix tree.
The PR title lives in the merge commit body (tide format).

Validation is mechanical, same as mine_cases.py for esbuild:

  1. reset a scratch worktree to the merge's FIRST PARENT
  2. overlay only the *_test.go / testdata files from the merge
  3. the narrowed `go test` must FAIL       (the bug reproduces)
  4. additionally checkout the source fix — the test must PASS

Only unit-testable areas are considered (pkg/, cmd/, plugin/, and staging/
source reachable through the vendor symlinks); test/ needs a live cluster.

  python3 mine_k8s.py <k8s-clone> <scratch-worktree-dir> [max_cases] [since]
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
MAX_CASES = int(sys.argv[3]) if len(sys.argv) > 3 else 10
SINCE = sys.argv[4] if len(sys.argv) > 4 else "2026-02-01"
OUT = Path(__file__).resolve().parent / "cases-k8s.json"

FIX_RE = re.compile(r"\bfix(es|ed)?\b|\bbug\b|regression|incorrect|wrong", re.I)
SKIP_RE = re.compile(
    r"typo|comment|docs?\b|changelog|readme|lint|gofmt|chore|\bci\b|bump|"
    r"dependenc|e2e|flak|revert|cleanup|refactor|deprecat|test[- ]only|"
    r"golangci|owners|feature.?gate.*(promot|graduat)|kep\b",
    re.I,
)
GENERATED_RE = re.compile(r"zz_generated|\.pb\.go$|api/openapi-spec|/generated/")
TESTABLE = ("pkg/", "cmd/", "plugin/")


def git(cwd, *args, timeout=900):
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True, timeout=timeout)


def is_overlay(path: str) -> bool:
    return path.endswith("_test.go") or "/testdata/" in path


def pr_title(sha: str, body: str) -> str:
    for line in body.splitlines():
        line = line.strip()
        if line and not line.startswith("Merge pull request"):
            return line
    r = git(CLONE, "log", "--no-merges", "--pretty=%s", f"{sha}^1..{sha}")
    subjects = r.stdout.strip().splitlines()
    return subjects[-1] if subjects else ""


def candidates():
    log = git(CLONE, "log", "--first-parent", "--merges", f"--since={SINCE}",
              "--pretty=%H\x01%cs\x01%b\x03", timeout=1200).stdout
    out = []
    for entry in log.split("\x03"):
        entry = entry.strip()
        if not entry or "\x01" not in entry:
            continue
        sha, date, body = entry.split("\x01", 2)
        title = pr_title(sha, body)
        if not title or not FIX_RE.search(title) or SKIP_RE.search(title):
            continue
        diff = git(CLONE, "diff", "--numstat", f"{sha}^1", sha).stdout
        overlay, code, codelines, bad = [], [], 0, False
        for line in diff.splitlines():
            parts = line.split("\t")
            if len(parts) != 3:
                continue
            add, rm, path = parts
            n = (0 if add == "-" else int(add)) + (0 if rm == "-" else int(rm))
            if not path.endswith(".go") and "/testdata/" not in path:
                continue  # OWNERS, docs, … — irrelevant to build and test
            if GENERATED_RE.search(path) or path.startswith("vendor/"):
                bad = True
                break
            if is_overlay(path):
                if not path.startswith(TESTABLE):
                    bad = True  # regression test not unit-runnable (test/e2e, staging)
                    break
                overlay.append(path)
            else:
                if not path.startswith(TESTABLE + ("staging/",)):
                    bad = True
                    break
                code.append(path)
                codelines += n
        if bad or not overlay or not (1 <= len(code) <= 3) or not (0 < codelines <= 80):
            continue
        out.append({"sha": sha, "date": date, "subject": title,
                    "overlay": overlay, "code": code, "codelines": codelines})
    return out


def test_pkgs(overlay):
    pkgs = set()
    for f in overlay:
        d = Path(f).parent
        while d.name == "testdata":
            d = d.parent
        pkgs.add("./" + d.as_posix() + "/")
    return sorted(pkgs)


def run_tests(pkgs, timeout=1200):
    r = subprocess.run(["go", "test", *pkgs], cwd=WT, capture_output=True, text=True, timeout=timeout)
    return r.returncode == 0


def reset_overlay(case):
    git(WT, "reset", "--hard", case["parent"])
    git(WT, "clean", "-fdx", "-e", ".gdbg")
    git(WT, "checkout", case["sha"], "--", *case["overlay"])


def validate(case) -> bool:
    parent = git(CLONE, "rev-parse", case["sha"] + "^1").stdout.strip()
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
        if per_pkg[pkg_key] >= 2:
            continue
        ok = validate(c)
        print(f"  {c['sha'][:10]} {c['date']} {'OK ' if ok else 'no '} {c['subject'][:70]}", flush=True)
        if ok:
            kept.append({
                "sha": c["sha"], "parent": c["parent"], "date": c["date"],
                "bug": c["subject"], "overlay": c["overlay"], "code": c["code"],
                "codelines": c["codelines"], "pkgs": c["pkgs"],
            })
            per_pkg[pkg_key] += 1
            OUT.write_text(json.dumps(kept, indent=2) + "\n")
        if len(kept) >= MAX_CASES:
            break
    OUT.write_text(json.dumps(kept, indent=2) + "\n")
    print(f"wrote {len(kept)} cases to {OUT}")


if __name__ == "__main__":
    main()
