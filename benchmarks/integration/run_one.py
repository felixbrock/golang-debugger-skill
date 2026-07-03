#!/usr/bin/env python3
"""One integration-benchmark run: a cross-service contract bug where the
other service's source is unavailable, so reading cannot reveal the cause.

  python3 run_one.py <without|with> <idx>

Copies the task to an isolated dir under /tmp, runs Claude Code headless on
the fix prompt (with = go-debugger skill installed + proof-of-observation
requirement; without = plain prompt), then restores the pristine tests and
binary and checks `go test ./...`. Records how the agent observed reality:
debugger calls, live probes of the service (curl etc.), and binary
inspection. Prints one JSON line.

Env: GDBG_HERMETIC=<config dir> for a clean agent config (recommended).
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
TASK = HERE / "task"
SKILL = HERE.parent.parent / "skill" / "go-debugger" / "SKILL.md"
OUT = Path("/tmp/gdbg-bench-integration")

GDBG_NOTE = """

Requirement: diagnose at runtime before editing. This project ships `gdbg`, a
runtime debugger for Go (run `gdbg` for usage; see .claude/skills/go-debugger).
Before your first edit, observe the failing flow under it —

    cd billing && gdbg launch --test . --break client.go:<line> -- -run TestInvoiceIntegration
    gdbg vars / gdbg eval <expr> / gdbg step over

— and quote the observed runtime values that identify the root cause in your
answer. Do not edit any file before you have observed and quoted those values."""

PROBE_RE = re.compile(r"curl|wget|bin/rates|/rate\?|http\.Get|nc \d|python.*urllib|python.*requests")
BINSPECT_RE = re.compile(r"(strings|objdump|hexdump|xxd|grep[^|]*-a)\s[^|]*rates")


def main() -> None:
    cond, idx = sys.argv[1], sys.argv[2]
    OUT.mkdir(exist_ok=True)
    work = OUT / f"{cond}-{idx}"
    if work.exists():
        shutil.rmtree(work)
    shutil.copytree(TASK, work)
    if not (work / "bin" / "rates").exists():
        raise SystemExit("bin/rates missing — build it: (cd rates-src && go build -o ../task/bin/rates .)")
    prompt = (TASK / "PROMPT.md").read_text()
    if cond == "with":
        d = work / ".claude" / "skills" / "go-debugger"
        d.mkdir(parents=True, exist_ok=True)
        shutil.copy(SKILL, d / "SKILL.md")
        prompt += GDBG_NOTE

    env = dict(os.environ, PATH=f"{Path.home()}/.local/bin:{Path.home()}/go/bin:" + os.environ["PATH"])
    if os.environ.get("GDBG_HERMETIC"):
        env["CLAUDE_CONFIG_DIR"] = os.environ["GDBG_HERMETIC"]
    start = time.monotonic()
    try:
        p = subprocess.run(
            ["claude", "-p", prompt, "--model", "opus", "--effort", "medium",
             "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"],
            cwd=work, capture_output=True, text=True, timeout=1800, env=env)
        out = p.stdout
    except subprocess.TimeoutExpired:
        out = ""
    wall = round(time.monotonic() - start, 1)
    (work / "transcript.jsonl").write_text(out)

    gdbg_calls = probe_calls = binspect_calls = 0
    tokens = None
    for line in out.splitlines():
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        m = ev.get("message", {})
        for b in (m.get("content") or []) if isinstance(m.get("content"), list) else []:
            if isinstance(b, dict) and b.get("type") == "tool_use" and b.get("name") == "Bash":
                cmd = str((b.get("input") or {}).get("command", ""))
                if cmd.strip().startswith("gdbg") or " gdbg " in cmd:
                    gdbg_calls += 1
                if PROBE_RE.search(cmd):
                    probe_calls += 1
                if BINSPECT_RE.search(cmd):
                    binspect_calls += 1
        if ev.get("type") == "result":
            u = ev.get("usage", {})
            tokens = sum(v for k, v in u.items() if k.endswith("_tokens") and isinstance(v, int))

    # kill leftover processes first (a running bin/rates blocks the restore),
    # then restore pristine tests + binary so tampering can't fake a pass
    subprocess.run(["pkill", "-f", "gdbg __daemon"], capture_output=True)
    subprocess.run(["pkill", "-f", "dlv "], capture_output=True)
    subprocess.run(["pkill", "-f", "bin/rates"], capture_output=True)
    time.sleep(0.5)
    for rel in ("billing/invoice_test.go", "billing/integration_test.go", "bin/rates"):
        (work / rel).unlink(missing_ok=True)
        shutil.copy(TASK / rel, work / rel)
    (work / "bin" / "rates").chmod(0o755)
    passed = subprocess.run(["go", "test", "./..."], cwd=work / "billing",
                            capture_output=True, timeout=300, env=env).returncode == 0
    subprocess.run(["pkill", "-f", "bin/rates"], capture_output=True)
    print(json.dumps({"condition": cond, "idx": idx, "passed": passed,
                      "gdbg_calls": gdbg_calls, "probe_calls": probe_calls,
                      "binary_inspection": binspect_calls,
                      "tokens": tokens, "wall_s": wall}))


if __name__ == "__main__":
    main()
