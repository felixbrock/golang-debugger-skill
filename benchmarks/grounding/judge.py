#!/usr/bin/env python3
"""Score one benchmark transcript for grounding quality.

Condenses a Claude Code stream-json transcript (assistant text, tool calls,
truncated tool results) and asks a judge model to score it: how many runtime
facts were actually observed vs asserted, whether the debugger's observations
causally drove the fix or merely decorated a conclusion reached by reading
("grounding theater"), and friction. Prints one JSON line.

  python3 judge.py <transcript.jsonl> <label>

Env: JUDGE_MODEL (default opus). Uses `claude -p`; run with a pristine
CLAUDE_CONFIG_DIR for reproducibility.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

MAX_TEXT = 4000      # per assistant text block
MAX_INPUT = 500      # per tool_use input
MAX_RESULT = 900     # per tool_result
MAX_TOTAL = 120_000  # condensed transcript budget (chars)

RUBRIC = """You are auditing a coding agent's debugging transcript for GROUNDING
QUALITY. The agent was fixing a failing Go test. Below is the condensed
transcript: the agent's own text, its tool calls, and truncated tool outputs.

Score it strictly. Definitions:

- A "runtime observation" is a concrete fact about program behavior the agent
  actually obtained in this session from execution output it can see: a quoted
  test failure value, a debugger stop, `vars`/`eval` output, a trace row.
- An "unverified runtime claim" is a statement about runtime behavior or
  concrete values ("this returns [7,9]", "sum is reset to 0 here at iteration
  2") asserted as fact when no execution output in the transcript shows it and
  it is not explicitly marked as inference/hypothesis.
- "causal_link" is true only if at least one debugger-obtained observation
  DETERMINED or CHANGED the diagnosis or fix — i.e., before that observation
  the agent had not yet committed to the root cause, or the observation
  contradicted/refined its hypothesis. Confirming an already-stated conclusion
  does NOT count.
- "theater" is true if the agent used the debugger but its conclusion was
  reached by reading before (or independent of) the observations — e.g. it
  states the bug from the source and then runs the debugger to comply with a
  policy or to double-check.

Return ONLY a JSON object, no prose, with exactly these keys:
{
  "grounded_observations": <int, distinct runtime facts obtained from execution output>,
  "debugger_observations": <int, subset of the above that came from the debugger (gdbg), not plain test output>,
  "unverified_runtime_claims": <int>,
  "debugger_used": <bool>,
  "debugger_calls": <int, number of gdbg invocations attempted>,
  "debugger_errors": <int, gdbg invocations that returned an error/failed>,
  "root_cause_source": "reading" | "debugger" | "test_output" | "mixed",
  "causal_link": <bool>,
  "theater": <bool>,
  "note": "<one sentence justifying causal_link/theater>"
}

TRANSCRIPT:
"""


def clip(s: str, n: int) -> str:
    s = str(s)
    return s if len(s) <= n else s[:n] + f"…[+{len(s)-n} chars]"


def condense(path: Path) -> str:
    parts = []
    for line in path.read_text().splitlines():
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        t = ev.get("type")
        msg = ev.get("message", {})
        content = msg.get("content") if isinstance(msg.get("content"), list) else []
        if t == "assistant":
            for b in content:
                if not isinstance(b, dict):
                    continue
                if b.get("type") == "text" and b.get("text", "").strip():
                    parts.append("AGENT: " + clip(b["text"], MAX_TEXT))
                elif b.get("type") == "tool_use":
                    parts.append(f"TOOL {b.get('name')}: " + clip(json.dumps(b.get("input", {})), MAX_INPUT))
        elif t == "user":
            for b in content:
                if isinstance(b, dict) and b.get("type") == "tool_result":
                    c = b.get("content")
                    text = c if isinstance(c, str) else " ".join(
                        x.get("text", "") for x in c if isinstance(x, dict)) if isinstance(c, list) else ""
                    err = " (ERROR)" if b.get("is_error") else ""
                    parts.append(f"RESULT{err}: " + clip(text, MAX_RESULT))
        elif t == "result":
            parts.append("FINAL: " + clip(ev.get("result", ""), 2000))
    out = "\n".join(parts)
    if len(out) > MAX_TOTAL:
        head, tail = int(MAX_TOTAL * 0.65), int(MAX_TOTAL * 0.3)
        out = out[:head] + f"\n…[{len(out)-head-tail} chars elided]…\n" + out[-tail:]
    return out


def main() -> None:
    path, label = Path(sys.argv[1]), sys.argv[2]
    prompt = RUBRIC + condense(path)
    model = os.environ.get("JUDGE_MODEL", "opus")
    p = subprocess.run(["claude", "-p", prompt, "--model", model, "--effort", "medium",
                        "--output-format", "json"],
                       capture_output=True, text=True, timeout=900)
    data = json.loads(p.stdout)
    text = data.get("result", "")
    # the judge may wrap JSON in a code fence
    text = text.strip().removeprefix("```json").removeprefix("```").removesuffix("```").strip()
    start, end = text.find("{"), text.rfind("}")
    verdict = json.loads(text[start:end + 1])
    verdict["label"] = label
    verdict["transcript"] = str(path)
    print(json.dumps(verdict))


if __name__ == "__main__":
    main()
