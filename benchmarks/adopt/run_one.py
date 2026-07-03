#!/usr/bin/env python3
"""One prompting-adoption run: does a strong CLAUDE.md make the agent use gdbg?

  python3 run_one.py <strong|control|gate|prompt> <idx> [task]

Copies the task to an isolated dir, writes the condition's CLAUDE.md, makes the
go-debugger skill available in both, runs Claude Code headless (Opus, medium
effort) on the fix task, then reports whether it used gdbg, how many times,
tokens, wall time, and whether `go test` passes. Prints one JSON line.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path

BENCH = Path(__file__).resolve().parent.parent
TASK = BENCH / "tasks" / (sys.argv[3] if len(sys.argv) > 3 else "accumulator")
SKILL = BENCH.parent / "skill" / "go-debugger" / "SKILL.md"
# Workdirs must live OUTSIDE the repo: a workdir under the repo makes the
# agent resolve the repo as its project root and inherit its CLAUDE.md,
# settings, and session memory — which contaminates the experiment.
OUT = Path(os.environ.get("GDBG_BENCH_OUT", "/tmp/gdbg-bench-results"))

STRONG_MD = """# Debugging policy (MANDATORY — read before touching code)

When a test fails or a value looks wrong, do NOT diagnose it by reading source,
grepping, or adding print/`fmt.Println` statements and re-running. That
guess-and-check loop is slow and unreliable and is NOT the approach to use here.

This project ships `gdbg`, a runtime debugger (run `gdbg` for usage). You are
required to OBSERVE the actual runtime values with it before editing anything:

    gdbg launch --test . --break <file>.go:<line> -- -run <TestName>
    gdbg vars                 # the real local values at that point
    gdbg eval <expr>          # one value
    gdbg step over            # watch execution advance
    gdbg trace --test . --break <file>.go:<line> --capture <vars> -- -run <TestName>

Workflow: run the failing test to see the assertion; set a breakpoint where the
suspect value is computed; launch that test under `gdbg`; read the ACTUAL values;
only then edit the code to correct what you observed. Breaking and inspecting is
your first move — not opening the file to read and reason.
"""

CONTROL_MD = """# Notes

A `go-debugger` skill (`gdbg`) is available in this project if you find it useful.
"""

# gate: same channel as strong (CLAUDE.md) but phrased as a verifiable review
# gate with a rationale, instead of a bare workflow mandate.
GATE_MD = """# Debugging policy — hard gate (enforced in review)

Fixes in this repo are only accepted when the root cause was OBSERVED at
runtime, not inferred from reading. Fixes justified only by reading the source
have repeatedly shipped confabulated root causes here, so review REJECTS any
edit that is not preceded, in the same session, by debugger evidence — even if
the tests pass afterward.

Concretely, before your FIRST edit you must:

1. Run the failing test under the project debugger `gdbg` (run `gdbg` for
   usage), breaking where the suspect value is computed:

       gdbg launch --test . --break <file>.go:<line> -- -run <TestName>
       gdbg vars        # or: gdbg trace --test . --break <loc> --capture <vars> -- -run <TestName>

2. Quote the observed runtime values (the STOP report, `vars` output, or trace
   table) in your reply, and state which observation identifies the root cause.

Reading the code to choose a good breakpoint is fine and encouraged. Editing
before you have quoted observed values is the one thing that fails review.
"""

PROMPT = ("The test in this Go module fails (`go test`). Find the root cause "
          "and fix the source so the test passes. Keep the change minimal and "
          "correct for the general case, not just the test input.")

# prompt: the mandate lives in the task prompt (user turn) instead of CLAUDE.md.
PROMPT_MANDATE = PROMPT + (
    " Requirement: diagnose at runtime before editing — run the failing test "
    "under the `gdbg` debugger (run `gdbg` for usage), break where the suspect "
    "value is computed, and quote the observed runtime values that identify the "
    "root cause in your answer. Do not edit any file before you have observed "
    "and quoted those values.")

CONDS = {
    "strong":  (STRONG_MD, PROMPT),
    "control": (CONTROL_MD, PROMPT),
    "gate":    (GATE_MD, PROMPT),
    "prompt":  (CONTROL_MD, PROMPT_MANDATE),
}


def main() -> None:
    cond, idx = sys.argv[1], sys.argv[2]
    OUT.mkdir(exist_ok=True)
    work = OUT / f"{TASK.name}-{cond}-{idx}"
    if work.exists():
        shutil.rmtree(work)
    shutil.copytree(TASK, work)
    d = work / ".claude" / "skills" / "go-debugger"
    d.mkdir(parents=True, exist_ok=True)
    shutil.copy(SKILL, d / "SKILL.md")
    claude_md, prompt = CONDS[cond]
    (work / "CLAUDE.md").write_text(claude_md)

    env = dict(os.environ, PATH=f"{Path.home()}/.local/bin:{Path.home()}/go/bin:" + os.environ["PATH"])
    # GDBG_HERMETIC=<dir>: run with a pristine user config (no global
    # CLAUDE.md, personal skills, plugins, or MCP servers). The dir needs a
    # copy of .credentials.json. Keeps host machine config out of the
    # experiment — see FINDINGS.md "Methodology hazard".
    if os.environ.get("GDBG_HERMETIC"):
        env["CLAUDE_CONFIG_DIR"] = os.environ["GDBG_HERMETIC"]
    start = time.monotonic()
    try:
        p = subprocess.run(
            ["claude", "-p", prompt, "--model", "opus", "--effort", "medium",
             "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"],
            cwd=work, capture_output=True, text=True, timeout=1500, env=env)
        out = p.stdout
    except subprocess.TimeoutExpired:
        out = ""
    wall = round(time.monotonic() - start, 1)
    (work / "transcript.jsonl").write_text(out)

    gdbg_calls = 0
    first_tool = None
    tokens = None
    for line in out.splitlines():
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        m = ev.get("message", {})
        for b in (m.get("content") or []) if isinstance(m.get("content"), list) else []:
            if isinstance(b, dict) and b.get("type") == "tool_use":
                name = b.get("name")
                cmd = str((b.get("input") or {}).get("command", ""))
                is_gdbg = name == "Bash" and (cmd.strip().startswith("gdbg") or " gdbg " in cmd)
                if first_tool is None:
                    first_tool = "gdbg" if is_gdbg else name
                if is_gdbg:
                    gdbg_calls += 1
        if ev.get("type") == "result":
            u = ev.get("usage", {})
            tokens = sum(v for k, v in u.items() if k.endswith("_tokens") and isinstance(v, int))

    passed = subprocess.run(["go", "test", "./..."], cwd=work, capture_output=True,
                            timeout=300, env=env).returncode == 0
    subprocess.run(["pkill", "-f", "gdbg __daemon"], capture_output=True)
    subprocess.run(["pkill", "-f", "dlv "], capture_output=True)
    print(json.dumps({"task": TASK.name, "condition": cond, "idx": idx,
                      "used_gdbg": gdbg_calls > 0, "gdbg_calls": gdbg_calls,
                      "first_tool": first_tool, "tokens": tokens,
                      "wall_s": wall, "passed": passed}))


if __name__ == "__main__":
    main()
