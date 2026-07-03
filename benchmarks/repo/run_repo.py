#!/usr/bin/env python3
"""Tier-2 benchmark: real fixed esbuild bugs, reset to before the fix.

For each case in cases.json the harness resets the scratch worktree to the
fix's PARENT commit, overlays only the regression test/snapshots from the fix,
confirms the narrowed test is red, then runs Claude Code headless to re-derive
the fix — once without gdbg (pure agent) and once with it (skill installed and
runtime observation required in the prompt; a passive availability note yields
~0% adoption, see ../FINDINGS.md, and this experiment measures benefit-when-
used). Before verification the overlay is re-checked-out so editing the test
or snapshots cannot fake a pass.

  python3 run_repo.py <esbuild-clone> <scratch-worktree-dir> [--cases N] [--conditions without,with]
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


def git(cwd, *args):
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True)


class Bench:
    def __init__(self, clone: Path, wt: Path):
        self.clone, self.wt = clone, wt
        if not wt.exists():
            r = git(clone, "worktree", "add", "--detach", str(wt), "HEAD")
            if r.returncode != 0:
                raise SystemExit(f"worktree add failed: {r.stderr}")

    def reset(self, case, with_skill: bool):
        git(self.wt, "reset", "--hard", case["parent"])
        git(self.wt, "clean", "-fdx")
        git(self.wt, "checkout", case["sha"], "--", *case["overlay"])
        if with_skill:
            d = self.wt / ".claude" / "skills" / "go-debugger"
            d.mkdir(parents=True, exist_ok=True)
            (d / "SKILL.md").write_text(SKILL.read_text())

    def verify(self, case, timeout=420):
        # re-checkout the overlay first so a tampered test/snapshot can't pass
        git(self.wt, "checkout", case["sha"], "--", *case["overlay"])
        r = subprocess.run(["go", "test", *case["pkgs"]], cwd=self.wt,
                           capture_output=True, text=True, timeout=timeout)
        return r.returncode == 0

    def run_agent(self, prompt: str, transcript: Path):
        env = dict(os.environ,
                   PATH=f"{Path.home()}/.local/bin:{Path.home()}/go/bin:" + os.environ["PATH"])
        try:
            p = subprocess.run(
                ["claude", "-p", prompt, "--model", "opus", "--effort", "medium",
                 "--output-format", "stream-json", "--verbose",
                 "--dangerously-skip-permissions"],
                cwd=self.wt, capture_output=True, text=True, timeout=2700, env=env)
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


def one_run(bench: Bench, case, cond, outdir: Path):
    bench.reset(case, with_skill=cond == "with")
    baseline_red = not bench.verify(case)
    prompt = BASE_PROMPT.format(pkgs=" ".join(case["pkgs"]))
    if cond == "with":
        prompt += GDBG_NOTE.format(pkg=case["pkgs"][0])
    start = time.monotonic()
    info = bench.run_agent(prompt, outdir / f"{case['sha'][:10]}-{cond}.jsonl")
    wall = round(time.monotonic() - start, 1)
    passed = bench.verify(case)
    # the next run's reset wipes .gdbg/ — keep this run's usage log
    usage = bench.wt / ".gdbg" / "usage.jsonl"
    if usage.exists():
        shutil.copy(usage, outdir / f"{case['sha'][:10]}-{cond}-usage.jsonl")
    subprocess.run(["pkill", "-f", "gdbg __daemon"], capture_output=True)
    subprocess.run(["pkill", "-f", "dlv "], capture_output=True)
    return {"case": case["sha"][:10], "bug": case["bug"][:60], "cond": cond,
            "baseline_red": baseline_red, "passed": passed, "wall_s": wall, **info}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("clone")
    ap.add_argument("worktree")
    ap.add_argument("--cases", type=int, default=0)
    ap.add_argument("--skip", type=int, default=0, help="skip the first N cases (resume)")
    ap.add_argument("--conditions", default="without,with")
    a = ap.parse_args()
    cases = json.loads((ROOT / "cases.json").read_text())
    cases = cases[a.skip:]
    if a.cases:
        cases = cases[: a.cases]
    bench = Bench(Path(a.clone).resolve(), Path(a.worktree).resolve())
    outdir = Path(os.environ.get("GDBG_REPO_OUT", "/tmp/gdbg-bench-repo-results"))
    outdir.mkdir(exist_ok=True)

    rows = []
    if a.skip and (ROOT / "runs.json").exists():
        rows = json.loads((ROOT / "runs.json").read_text())
    for case in cases:
        for cond in a.conditions.split(","):
            print(f"[{time.strftime('%H:%M:%S')}] {case['sha'][:10]} / {cond} — {case['bug'][:60]}", flush=True)
            row = one_run(bench, case, cond, outdir)
            print(f"  -> red={row['baseline_red']} passed={row['passed']} "
                  f"gdbg={row['gdbg_calls']} tokens={row['tokens']} wall={row['wall_s']}s", flush=True)
            rows.append(row)
            (ROOT / "runs.json").write_text(json.dumps(rows, indent=2) + "\n")

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
