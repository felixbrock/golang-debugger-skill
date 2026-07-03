#!/usr/bin/env python3
"""Aggregate grounding judgments (results.jsonl) into per-group means."""

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

groups: dict[str, list[dict]] = defaultdict(list)
for r in rows:
    tier, cond, _ = r["label"].split(":", 2)
    groups[f"{tier}:{cond}"].append(r)


def mean(rs, key):
    vals = [r[key] for r in rs if isinstance(r.get(key), (int, float))]
    return sum(vals) / len(vals) if vals else 0.0


print(f"{len(rows)} judged transcripts\n")
for g in sorted(groups):
    rs = groups[g]
    dbg = [r for r in rs if r.get("debugger_used")]
    print(f"{g}  (n={len(rs)})")
    print(f"  grounded observations/run: {mean(rs,'grounded_observations'):.1f} "
          f"(from debugger: {mean(rs,'debugger_observations'):.1f})")
    print(f"  unverified runtime claims/run: {mean(rs,'unverified_runtime_claims'):.1f}")
    if dbg:
        causal = sum(bool(r.get("causal_link")) for r in dbg)
        theater = sum(bool(r.get("theater")) for r in dbg)
        errs = mean(dbg, "debugger_errors")
        print(f"  debugger users: {len(dbg)}/{len(rs)}  causal link: {causal}/{len(dbg)}  "
              f"theater: {theater}/{len(dbg)}  errors/run: {errs:.1f}")
    src = defaultdict(int)
    for r in rs:
        src[r.get("root_cause_source", "?")] += 1
    print(f"  root cause source: {dict(sorted(src.items()))}")
    print()

print("causal-link cases (observation drove the diagnosis):")
for r in rows:
    if r.get("causal_link"):
        print(f"  {r['label']}: {r.get('note','')[:140]}")
