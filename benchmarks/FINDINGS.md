# What the adoption benchmarks showed (Go replication)

A replication of the rust-debugger-skill adoption experiment, run 2026-07-02:
same protocol (two planted-bug tasks × strong-vs-control `CLAUDE.md` × 5
headless runs), Claude Code 2.1.198, `claude-opus-4-8`, medium effort, `gdbg`
+ Delve 1.27 on Go 1.26.

## Both experiments, side by side

Go (hermetic runs — pristine user config, isolated workdirs; see the
methodology notes below):

| bug | strong adoption | control adoption | strong tokens | control tokens | pass (both) |
|---|---|---|---|---|---|
| accumulator (easy) | 1/5 | 0/5 | 142k | 108k (**1.31× cheaper**) | 5/5 = 5/5 |
| rpncalc (runtime-opaque) | 3/5 | 0/5 | 191k | 128k (**1.49× cheaper**) | 5/5 = 5/5 |

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

## Mandates are advisory, artifacts are not — and ambient context is a confound

The bare "MANDATORY, observe with the debugger before editing" `CLAUDE.md`
that flipped Rust adoption to 5/5 achieved 3/5 here — and probing WHY led to
the sharpest result of the adoption study. Both studies used the same model
(the Rust repo's committed results show `claude-opus-4-8`, `--model opus
--effort medium`, identical to ours), so we tested the remaining variable:
the **host machine's user-level Claude config** (global CLAUDE.md, personal
skills, plugins, MCP servers), which loads into every nested benchmark run.
Re-running cells hermetically (`CLAUDE_CONFIG_DIR` pointed at a pristine
config; `GDBG_HERMETIC` in the harness) as well as under the host config
(`results-followup.jsonl`, `results-hermetic.jsonl`):

| condition | ambient context | adoption | mean tokens |
|---|---|---|---|
| control | hermetic | 0/5 | 108k |
| control | host personal config | 0/5 | 120k |
| strong | hermetic | **1/5** | 142k |
| strong | host personal config | **3/5** | 158k |
| strong | host config + stale session memory | **0/5** | 122k |
| gate | hermetic | **5/5** | 165k |
| gate | host personal config | **5/5** | 249k |
| *strong, Rust task (their machine)* | *unknown host config* | *5/5* | *386k* |
| *strong, Rust task (our machine)* | *hermetic* | ***4/5*** | *187k* |

Three regimes, cleanly separated:

- **Passive notes are robustly 0/5** in every environment, both studies.
- **Bare workflow mandates are unstable**: 0/5, 1/5, 3/5, 5/5 across
  ambient-context variations of the *same* prompt on the *same* model. The
  Rust study's 5/5 is best read as one more draw from this noisy
  distribution on one more uncontrolled machine — not a property of the
  language or the model.
- **Artifact-gates are robustly 5/5** everywhere — including the clean
  room, where they are also cheapest (165k, only 1.53× the hermetic
  control; the host config had inflated gate runs to 249k and 3–6 gdbg
  calls vs 1–2 hermetic).

The host config also taxes everything uniformly: hermetic control runs cost
108k vs 120k under the personal config (~11% of every measurement was the
host's skills/plugins/MCP being carried in the system prompt). Ratios
within a study survive this; absolute token comparisons across machines do
not.

### Cross-language control: the mandate is not technology-independent

To separate language from machine, we ran the **Rust study's own harness and
accumulator task on our machine, hermetically** (rustup + rust-analyzer +
lldb-dap installed locally, rdbg built from their repo, pristine
`CLAUDE_CONFIG_DIR`; raw data in `results-rust-hermetic.jsonl`). Everything
equal except the language and debugger:

- Go strong, hermetic, this machine: **1/5** adoption (142k mean tokens)
- Rust strong, hermetic, this machine: **4/5** adoption (187k)
- Rust strong, their machine, their config: 5/5 (386k)
- Controls: 0/5 in all three settings; every run passed everywhere.

Same model, same machine, same clean config, same prompt structure,
near-identical planted bug — and the agent reaches for the debugger 4× more
often in Rust. Pooled across environments the pattern holds (Rust strong
9/10 vs Go strong 4/10). With n=5 cells this is suggestive rather than
conclusive, but the direction is consistent: **the "reading is enough"
override that neuters bare mandates fires more often in Go than in Rust** —
plausibly because the model trusts its Go reading more. Adoption is a
three-way function of instructions, ambient environment, *and* language;
only the verifiable-artifact gate has been immune to all of them.

The clean-room Rust runs also isolate what their host config had added:
adopting runs there needed 2–3 rdbg calls at ~187k tokens versus 6–9 calls
at ~386k in the original study — ambient context roughly doubled the
debugging cost, mirroring (at larger scale) the gate-run inflation we
measured on our own machine.

The gate variant phrases the policy as a review gate with a reason: *fixes
are rejected unless the reply quotes runtime values observed before the
first edit, because read-only fixes have shipped confabulated root causes*.
Every gate run complied — several noted "the bug is visible on reading, but
the policy requires runtime observation" and quoted a `gdbg trace` table
showing the odd elements being appended.

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
| pipeline | control | 0/5 | 0 | 162k | 57s | **5/5** |
| pipeline | gate | 5/5 | 3.2 | 297k (1.83×) | 90s | 5/5 |
| panic_deep | control | 0/5 | 0 | 136k | 28s | **5/5** |
| panic_deep | gate | 5/5 | 4.8 | 241k (1.77×) | 54s | 5/5 |

(Hermetic numbers; the earlier host-config pass produced the same picture at
slightly higher cost — 351k/260k gate means, `results-hard.jsonl`.)

The read-loop did not crack. Control transcripts show the same shape every
time: read all eight files once, edit the buggy line, run the test — no
print statements, no second iteration, no wrong turns. Opus 4.8 spot-reads a
one-character boundary bug in 350 unfamiliar lines reliably, and resolves a
panic two modules from its cause without ever running the code.

So across four tasks spanning easy → runtime-opaque → multi-file-subtle →
distant-cause-panic (60+ runs), the debugger never improved pass rate and
always cost 1.3–1.8× tokens (hermetic; up to 2.1× under host config). Every
tier-1 cell was subsequently reproduced hermetically — 45 clean-room runs,
no conclusion changed. At self-contained-module scale (≤ ~350 lines),
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

## Tier 2: real bugs in a real codebase (esbuild, 95k LOC)

The last regime where a token win was plausible: bugs whose surrounding
codebase is too large to just read. 12 real, merged esbuild bug fixes
(mined from history: `fix #NNNN` commits shipping a regression test plus a
≤60-line source fix, each mechanically validated red-at-parent /
green-with-fix — `repo/cases.json`). Per case the harness resets a worktree
to the fix's parent, overlays only the test/snapshots, confirms red, and
runs the agent to re-derive the fix; the overlay is re-checked-out before
verification so tampering with tests can't fake a pass. Conditions: pure
agent (`without`) vs gdbg mandated via the artifact-gate prompt (`with`) —
availability notes yield ~0% adoption, and this measures benefit-when-used.

| | without | with |
|---|---|---|
| solved | **12/12** | **12/12** |
| mean tokens | 708k | 1,280k (1.81×) |
| median tokens | 419k | 707k (1.69×) |
| mean wall | 173s | 227s |
| mean gdbg calls | 0 | 7.1 |

Still no crossing — but the texture changed:

- On the two cases where reading was most expensive (`e0755b46` parens for
  `new` expressions, 2.0M tokens without; `2b6452b5` `import =` under
  `es5`), the debugger arm reached **token parity and clearly better wall
  time** (280s vs 472s; 213s vs 270s). First sightings of the gap actually
  closing on individual bugs.
- The with-arm mean is dominated by one 6.2M-token run (`308ad745`, nested
  `var` renaming) — not tool friction (1 error in 24 gdbg calls) but a
  genuinely long session of conditional breakpoints and symbol-table evals.
  Excluding that case from both arms: 608k vs 829k (1.36×).
- gdbg held up operationally in a 95k-line codebase: agents set breakpoints
  in deep internals (`internal/renamer`), evaluated indexed expressions into
  real data structures, and traced across packages, with essentially no CLI
  friction.

This mirrors the Rust tier-2 experience (on a 1.7M-line repo the agent
never reached for rdbg and grep-fixed everything), with one upgrade: forced
adoption now *works* and is approaching break-even exactly where reading is
most expensive — without ever beating it.

## Bottom line

1. Passive availability still yields ~0% adoption, across languages and model
   versions.
2. Prompting controls adoption, with a gradient: bare workflow mandates are
   unstable — sensitive to ambient context (0–5/5 across environments on the
   same model and prompt) *and to language* (Rust 4/5 vs Go 1/5 on the same
   machine, hermetic) — while verifiable-artifact gates reach 5/5 in every
   environment tested, including a hermetic one.
3. Forced adoption still never beat reading on token cost at this task scale
   (best case ~1.3×, all cells 5/5 correct). The case for gdbg remains
   conditional: panics with unclear cause, state only visible at runtime,
   codebases too large to read — plus interactive/confirmation use.
