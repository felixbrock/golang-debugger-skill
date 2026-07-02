# What the adoption benchmarks showed (Go replication)

A replication of the rust-debugger-skill adoption experiment, run 2026-07-02:
same protocol (two planted-bug tasks × strong-vs-control `CLAUDE.md` × 5
headless runs), Claude Code 2.1.198, `claude-opus-4-8`, medium effort, `gdbg`
+ Delve 1.27 on Go 1.26.

## Both experiments, side by side

Go (clean runs, isolated workdirs — see the contamination note below):

| bug | strong adoption | control adoption | strong tokens | control tokens | pass (both) |
|---|---|---|---|---|---|
| accumulator (easy) | 3/5 | 0/5 | 158k | 120k (**1.32× cheaper**) | 5/5 = 5/5 |
| rpncalc (runtime-opaque) | 3/5 | 0/5 | 194k | 142k (**1.36× cheaper**) | 5/5 = 5/5 |

The original Rust numbers, for reference:

| bug | strong adoption | control adoption | strong tokens | control tokens | pass (both) |
|---|---|---|---|---|---|
| accumulator (easy) | 5/5 | 0/5 | 386k | 135k (**2.85× cheaper**) | 5/5 = 5/5 |
| rpncalc (runtime-opaque) | 4/5 | 0/5 | 278k | 153k (**1.82× cheaper**) | 5/5 = 5/5 |

## What replicated

- **Availability alone yields zero use.** Control adoption was 0/10 across
  both tasks — identical to Rust. A passive "skill available" note does
  nothing.
- **Adoption ≠ benefit.** Every cell passed 5/5; the debugger never paid for
  itself in tokens on bugs this size. The penalty is much smaller than in the
  Rust runs (~1.35× vs 1.8–2.9×), partly because Delve/gdbg friction was low
  in clean runs (adopting runs needed 1–2 gdbg calls) and partly because the
  current model uses the debugger surgically rather than exhaustively.

## What changed: mandates are now advisory, artifacts are not

The bare "MANDATORY, observe with the debugger before editing" `CLAUDE.md`
that flipped Rust adoption to 5/5 only achieved **3/5** here, on both tasks —
the current Opus treats it as advisory and sometimes decides reading is
enough ("The bug is clear from the source"), at control-level cost.

A follow-up on the easy bug (5 runs per condition, `results-followup.jsonl`)
shows adoption is still fully controllable — the bar just moved:

| condition | where the mandate lives | adoption | mean tokens |
|---|---|---|---|
| strong | CLAUDE.md, bare workflow mandate | 3/5 | 158k |
| **gate** | CLAUDE.md, *verifiable artifact + rationale* | **5/5** | 249k |
| **prompt** | the user-turn task prompt itself | **5/5** | 284k |
| control | one-line "skill available" note | 0/5 | 120k |

The gate variant phrases the policy as a review gate with a reason: *fixes
are rejected unless the reply quotes runtime values observed before the first
edit, because read-only fixes have shipped confabulated root causes*. Every
gate run complied — several noted "the bug is visible on reading, but the
policy requires runtime observation" and quoted a `gdbg trace` table showing
the odd elements being appended. Checkable artifact requirements and
user-turn instructions flip adoption; bare workflow prescriptions no longer
do.

## Methodology hazard: nested agents inherit the host project's context

The first pass of this experiment put benchmark workdirs **inside this
repo**. Nested `claude -p` runs then resolved the repo as their project root
and inherited its session memory — which at the time described a (stale)
`GOTOOLCHAIN` workaround and gdbg daemon failure modes. That contaminated
the runs in both directions:

- Two strong runs exported the stale `GOTOOLCHAIN` pin, dlv refused to
  start, and the runs burned 396k/439k tokens on recovery (inflating the
  strong mean to 273k, reported here originally as 1.92×).
- The failure-describing memory *suppressed* adoption elsewhere: the
  contaminated accumulator strong cell measured 0/5; re-run with isolated
  workdirs it is 3/5.

The harness now creates workdirs under `/tmp/gdbg-bench-results` (no repo
ancestor), and the contaminated rows are archived in
`results-contaminated.jsonl`. If you replicate agent benchmarks with Claude
Code: isolate workdirs from any real project root, or your CLAUDE.md,
settings, and memory become uncontrolled variables.

The friction episode did yield a real fix: the gdbg daemon used to keep the
environment of whichever client spawned it, so one bad env poisoned every
later launch. `launch`/`trace` now use the requesting client's environment.

## The performance experiment: can the debugger *win*?

Every cell above passed 5/5, so the earlier experiments only ever measured
overhead. Two harder tasks were built specifically so reading should
struggle, then run with-debugger (gate) vs pure read-loop (control), 5 runs
each (`results-hard.jsonl`):

- **pipeline** — a 6-stage log-analytics pipeline (~350 lines, 8 files); the
  bug is one reversed comparison at a window boundary (`w.End >= ev.TS` for
  `ev.TS < w.End`), and the test fails on an end-to-end aggregate that points
  at nothing (`{Windows:4 … Score:4}` vs `{Windows:6 … Score:3}`).
- **panic_deep** — an order-fulfillment module that panics
  (`index out of range [8] with length 8`) two files away from the cause: a
  branch in `Catalog.Add` records the index *after* appending, so backordered
  items are off by one (and silently resolve to the wrong item when not
  last).

| task | condition | adoption | mean calls | mean tokens | mean wall | passed |
|---|---|---|---|---|---|---|
| pipeline | control | 0/5 | 0 | 165k | 46s | **5/5** |
| pipeline | gate | 5/5 | 5.2 | 351k (2.1×) | 84s | 5/5 |
| panic_deep | control | 0/5 | 0 | 141k | 26s | **5/5** |
| panic_deep | gate | 5/5 | 4.4 | 260k (1.8×) | 54s | 5/5 |

The read-loop did not crack. Control transcripts show the same shape every
time: read all eight files once, edit the buggy line, run the test — no
print statements, no second iteration, no wrong turns. Opus 4.8 spot-reads a
one-character boundary bug in 350 unfamiliar lines reliably, and resolves a
panic two modules from its cause without ever running the code.

So across four tasks spanning easy → runtime-opaque → multi-file-subtle →
distant-cause-panic (60+ runs), the debugger never improved pass rate and
always cost 1.3–2.1× tokens. At self-contained-module scale (≤ ~350 lines),
reading is saturated for this model; there is no bug-difficulty dial at this
size that makes a debugger pay off. Where that leaves gdbg's value, in order
of evidence: (1) grounding — debugger runs quote observed values instead of
asserting simulated ones (the Rust grounding analysis found 3× more grounded
observations, and our gate transcripts match); (2) codebases too large or
expensive to read, where one paused inspection replaces many
build-and-print cycles — untested here, needs a tier-2 style harness on a
real repo; (3) failure modes reading can't reach in principle: state that
exists only under load, concurrency interleavings, external inputs; (4)
interactive use by humans.

## Bottom line

1. Passive availability still yields ~0% adoption, across languages and model
   versions.
2. Prompting still controls adoption, but the mechanism changed: this model
   generation overrides bare workflow mandates it judges inefficient (3/5),
   while verifiable-artifact gates and user-turn requirements reach 5/5.
3. Forced adoption still never beat reading on token cost at this task scale
   (best case ~1.3×, all cells 5/5 correct). The case for gdbg remains
   conditional: panics with unclear cause, state only visible at runtime,
   codebases too large to read — plus interactive/confirmation use.
