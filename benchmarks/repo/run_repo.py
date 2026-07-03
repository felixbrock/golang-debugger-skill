#!/usr/bin/env python3
"""Tier-2 benchmark: real fixed esbuild bugs, reset to before the fix.

Contamination-isolated, mirroring the rust-debugger-skill tsz method:

- each run gets a FRESH single-commit checkout of the fix's parent (built via
  `git archive` + `git init`), so the fix commit is not in the object store
  and `git log/show/blame` reveal nothing;
- WebSearch/WebFetch are disallowed, so the agent cannot look up the fix;
- only the regression test/snapshots from the fix are overlaid, and they are
  rewritten from the pristine source before verification so tampering cannot
  fake a pass;
- cases carry commit dates — use --since to keep only post-training-cutoff
  cases (e.g. --since 2026-02).

Conditions: `without` (plain agent) vs `with` (go-debugger skill installed
and runtime observation required in the prompt; a passive note yields ~0%
adoption, see ../FINDINGS.md — this measures benefit-when-used).

  python3 run_repo.py <esbuild-clone> [--cases N] [--since YYYY-MM]
                      [--conditions without,with] [--only sha1,sha2]
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent
SKILL = ROOT.parent.parent / "skill" / "go-debugger" / "SKILL.md"
WORKROOT = Path("/tmp/gdbg-bench-repo2")
OUTDIR = Path("/tmp/gdbg-bench-repo2-results")

BASE_PROMPT = """A regression test in this repository fails:

    go test {pkgs}

Diagnose the root cause and fix the source so that test passes. Do NOT modify
any *_test.go file or anything under snapshots/ — fix the source. Keep the
change minimal and correct for the general case; this is a real behavior
regression. The repository is large: only ever run the narrowed test command
above, never the full suite."""

GDBG_NOTE = """

Requirement: diagnose at runtime before editing. This project ships `gdbg`, a
runtime debugger (run `gdbg` for usage; see .claude/skills/go-debugger).
Before your first edit, run the failing test under it —

    gdbg launch --test {pkg} --break <file>:<line> -- -run <TestName>
    gdbg vars / gdbg eval <expr> / gdbg bt / gdbg trace … --capture <vars>

— and quote the observed runtime values that identify the root cause in your
answer. Do not edit any file before you have observed and quoted those values."""


def git(cwd, *args, check=False):
    r = subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True)
    if check and r.returncode != 0:
        raise SystemExit(f"git {' '.join(args)}: {r.stderr}")
    return r


class Bench:
    def __init__(self, clone: Path):
        self.clone = clone

    def _overlay(self, case, workdir: Path):
        for rel in case["overlay"]:
            dst = workdir / rel
            dst.parent.mkdir(parents=True, exist_ok=True)
            r = git(self.clone, "show", f"{case['sha']}:{rel}")
            dst.write_text(r.stdout)

    def build_workdir(self, case, with_skill: bool) -> Path:
        workdir = WORKROOT / f"{case['sha'][:10]}-{'with' if with_skill else 'without'}"
        if workdir.exists():
            shutil.rmtree(workdir)
        workdir.mkdir(parents=True)
        # single-commit checkout: tree of the parent, no history, no fix object
        archive = subprocess.run(["git", "archive", case["parent"]],
                                 cwd=self.clone, capture_output=True)
        subprocess.run(["tar", "-x"], cwd=workdir, input=archive.stdout, check=True)
        self._overlay(case, workdir)
        git(workdir, "init", "-q", check=True)
        git(workdir, "add", "-A", check=True)
        git(workdir, "-c", "user.email=bench@local", "-c", "user.name=bench",
            "commit", "-qm", "import", check=True)
        if with_skill:
            d = workdir / ".claude" / "skills" / "go-debugger"
            d.mkdir(parents=True, exist_ok=True)
            (d / "SKILL.md").write_text(SKILL.read_text())
        return workdir

    def verify(self, case, workdir: Path, timeout=420):
        self._overlay(case, workdir)  # tampered tests/snapshots can't fake a pass
        r = subprocess.run(["go", "test", *case["pkgs"]], cwd=workdir,
                           capture_output=True, text=True, timeout=timeout)
        return r.returncode == 0

    def run_agent(self, workdir: Path, prompt: str, transcript: Path):
        env = dict(os.environ,
                   PATH=f"{Path.home()}/.local/bin:{Path.home()}/go/bin:" + os.environ["PATH"])
        if os.environ.get("GDBG_HERMETIC"):
            env["CLAUDE_CONFIG_DIR"] = os.environ["GDBG_HERMETIC"]
        try:
            p = subprocess.run(
                ["claude", "-p", prompt, "--model", "opus", "--effort", "medium",
                 "--output-format", "stream-json", "--verbose",
                 "--disallowedTools", "WebSearch", "WebFetch",
                 "--dangerously-skip-permissions"],
                cwd=workdir, capture_output=True, text=True, timeout=2700, env=env)
            out = p.stdout
        except subprocess.TimeoutExpired:
            out = ""
        transcript.write_text(out)
        gdbg_calls, tokens, turns = 0, None, None
        for line in out.splitlines():
            try:
                ev = json.loads(line)
            except json.JSONDecodeError:
                continue
            m = ev.get("message", {})
            content = m.get("content") if isinstance(m.get("content"), list) else []
            for b in content:
                if isinstance(b, dict) and b.get("type") == "tool_use" and b.get("name") == "Bash":
                    cmd = str((b.get("input") or {}).get("command", ""))
                    if cmd.strip().startswith("gdbg") or " gdbg " in cmd:
                        gdbg_calls += 1
            if ev.get("type") == "result":
                u = ev.get("usage", {})
                tokens = sum(v for k, v in u.items() if k.endswith("_tokens") and isinstance(v, int))
                turns = ev.get("num_turns")
        return {"gdbg_calls": gdbg_calls, "tokens": tokens, "turns": turns, "timed_out": out == ""}


def one_run(bench: Bench, case, cond):
    workdir = bench.build_workdir(case, with_skill=cond == "with")
    baseline_red = not bench.verify(case, workdir)
    prompt = BASE_PROMPT.format(pkgs=" ".join(case["pkgs"]))
    if cond == "with":
        prompt += GDBG_NOTE.format(pkg=case["pkgs"][0])
    start = time.monotonic()
    info = bench.run_agent(workdir, prompt, OUTDIR / f"{case['sha'][:10]}-{cond}.jsonl")
    wall = round(time.monotonic() - start, 1)
    passed = bench.verify(case, workdir)
    usage = workdir / ".gdbg" / "usage.jsonl"
    if usage.exists():
        shutil.copy(usage, OUTDIR / f"{case['sha'][:10]}-{cond}-usage.jsonl")
    subprocess.run(["pkill", "-f", "gdbg __daemon"], capture_output=True)
    subprocess.run(["pkill", "-f", "dlv "], capture_output=True)
    return {"case": case["sha"][:10], "date": case.get("date", "?"),
            "bug": case["bug"][:60], "cond": cond,
            "baseline_red": baseline_red, "passed": passed, "wall_s": wall, **info}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("clone")
    ap.add_argument("--cases", type=int, default=0)
    ap.add_argument("--since", default="", help="keep only cases with date >= this (YYYY-MM)")
    ap.add_argument("--only", default="", help="comma-separated sha prefixes")
    ap.add_argument("--conditions", default="without,with")
    ap.add_argument("--out", default="runs-clean.json")
    a = ap.parse_args()
    cases = json.loads((ROOT / "cases.json").read_text())
    if a.since:
        cases = [c for c in cases if c.get("date", "") >= a.since]
    if a.only:
        keep = tuple(a.only.split(","))
        cases = [c for c in cases if c["sha"].startswith(keep)]
    if a.cases:
        cases = cases[: a.cases]
    bench = Bench(Path(a.clone).resolve())
    OUTDIR.mkdir(exist_ok=True)

    rows = []
    for case in cases:
        for cond in a.conditions.split(","):
            print(f"[{time.strftime('%H:%M:%S')}] {case['sha'][:10]} / {cond} — {case['bug'][:60]}", flush=True)
            row = one_run(bench, case, cond)
            print(f"  -> red={row['baseline_red']} passed={row['passed']} "
                  f"gdbg={row['gdbg_calls']} tokens={row['tokens']} wall={row['wall_s']}s", flush=True)
            rows.append(row)
            (ROOT / a.out).write_text(json.dumps(rows, indent=2) + "\n")

    valid = [r for r in rows if r["baseline_red"]]
    print("\n=== with vs without (means over red-baseline runs) ===")
    for cond in ("without", "with"):
        g = [r for r in valid if r["cond"] == cond]
        if not g:
            continue
        tok = [r["tokens"] for r in g if r["tokens"]]
        print(f"{cond:<9} solved {sum(r['passed'] for r in g)}/{len(g)}  "
              f"mean tokens {sum(tok)//max(len(tok),1):,}  "
              f"mean wall {sum(r['wall_s'] for r in g)/len(g):.0f}s  "
              f"mean gdbg calls {sum(r['gdbg_calls'] for r in g)/len(g):.1f}")


if __name__ == "__main__":
    main()
