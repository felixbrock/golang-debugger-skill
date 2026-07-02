#!/usr/bin/env python3
"""Aggregate results.jsonl (+ pilot lines, if passed as extra files) into the
side-by-side adoption table."""

from __future__ import annotations

import json
import sys
from collections import defaultdict
from pathlib import Path

rows = []
for path in sys.argv[1:] or ["results.jsonl"]:
    for line in Path(path).read_text().splitlines():
        line = line.strip()
        if line:
            rows.append(json.loads(line))

cells: dict[tuple[str, str], list[dict]] = defaultdict(list)
for r in rows:
    cells[(r["task"], r["condition"])].append(r)


def fmt_tokens(n: float) -> str:
    return f"{n / 1000:.0f}k"


print(f"{len(rows)} runs\n")
print("| bug | strong adoption | control adoption | strong tokens | control tokens | pass (both) |")
print("|---|---|---|---|---|---|")
for task in sorted({t for t, _ in cells}):
    s, c = cells[(task, "strong")], cells[(task, "control")]
    sa = sum(r["used_gdbg"] for r in s)
    ca = sum(r["used_gdbg"] for r in c)
    st = sum(r["tokens"] or 0 for r in s) / max(len(s), 1)
    ct = sum(r["tokens"] or 0 for r in c) / max(len(c), 1)
    ratio = st / ct if ct else 0
    sp = sum(r["passed"] for r in s)
    cp = sum(r["passed"] for r in c)
    print(f"| {task} | {sa}/{len(s)} | {ca}/{len(c)} | {fmt_tokens(st)} | "
          f"{fmt_tokens(ct)} (**{ratio:.2f}x cheaper**) | {sp}/{len(s)} = {cp}/{len(c)} |")

print("\nper-run detail:")
for r in sorted(rows, key=lambda r: (r["task"], r["condition"], int(r["idx"]))):
    print(f"  {r['task']:<12} {r['condition']:<8} #{r['idx']}  "
          f"gdbg_calls={r['gdbg_calls']:<3} first_tool={str(r['first_tool']):<10} "
          f"tokens={r['tokens']}  wall={r['wall_s']}s  passed={r['passed']}")
