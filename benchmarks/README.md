# Benchmarks

Will a coding agent *use* `gdbg` when it's available, and does forcing it pay
off? A replication of the
[rust-debugger-skill adoption experiment](https://github.com/mohsen1/rust-debugger-skill/blob/main/benchmarks/FINDINGS.md)
for Go. Runs make real API calls and cost money. Needs `gdbg` and `dlv` on
PATH and the `claude` CLI.

## The adoption experiment (`adopt/`)

Each task is a small Go module with a planted bug and a failing test. The
harness copies the task to an isolated dir, drops the `go-debugger` skill into
`.claude/skills/`, and writes one of two `CLAUDE.md` files:

- **strong** — a forceful debugging policy that mandates observing runtime
  values with `gdbg` before editing, and forbids the read/grep/print loop.
- **control** — a one-line note that the skill exists.

Then it runs Claude Code headless (`claude -p`, Opus, medium effort) on the
same fix prompt and records whether the agent invoked `gdbg`, how many times,
total tokens, wall time, and whether `go test` passes afterward.

```sh
python3 adopt/run_one.py strong 0 rpncalc     # one run, prints one JSON line
sh adopt/run_all.sh                           # the full 2x2x5 grid
python3 adopt/summarize.py adopt/results.jsonl
```

## Tasks

- `accumulator` (easy) — averages the wrong parity of numbers (`x%2 == 1`
  keeps odds, not evens). Readable straight from the source.
- `rpncalc` (runtime-opaque) — an RPN calculator with swapped operands on the
  non-commutative ops (`b-a`, `b/a`). The wrong final value doesn't point at
  the fault; watching the stack at a breakpoint does.
- `pipeline` (multi-file subtle) — a 6-stage log-analytics pipeline where one
  reversed boundary comparison shifts events between windows; the test fails
  on an end-of-pipeline aggregate that points at nothing.
- `panic_deep` (distant cause) — an order-fulfillment module that panics two
  files away from the off-by-one that corrupts its index.

Add a task by dropping a module under `tasks/<name>/` with a failing test and
a `PROMPT.md`.

## Tier 2 — real bugs in esbuild (`repo/`)

SWE-bench style, on [esbuild](https://github.com/evanw/esbuild) (~95k lines
of Go). Cases are merged bug-fix commits that shipped a regression test,
mined from history and mechanically validated (red at the parent commit with
the test overlaid, green with the fix):

```sh
git clone --filter=blob:none https://github.com/evanw/esbuild /path/to/esbuild
python3 repo/mine_cases.py /path/to/esbuild /tmp/gdbg-bench-esbuild 12
python3 repo/run_repo.py   /path/to/esbuild /tmp/gdbg-bench-esbuild
```

The harness resets the worktree to the fix's parent, overlays only the
test/snapshots, confirms red, runs the agent (without gdbg vs with it
mandated), re-checks-out the overlay so test tampering can't fake a pass,
and verifies. Results in `repo/runs.json`.

## Terminology

The setup in one sentence: give Claude a Go project with a bug and a failing
test, say "fix it", repeat N times per condition, and record whether it used
the debugger, what it cost, and whether the test passes afterward. The only
thing that varies between conditions is *how the debugger is suggested*.

**Conditions** (what differs between run groups):

- **control** — the baseline. The project contains only a one-line note that
  a debugger (`gdbg`) is available. No pressure; measures what the agent
  does naturally. (Observed: it never uses the debugger.)
- **strong** — a forceful `CLAUDE.md` in the project: "MANDATORY: do NOT
  diagnose by reading or adding prints — observe runtime values with the
  debugger first." An order, but with nothing verifiable attached. The
  agent often overrides it when it judges reading sufficient.
- **gate** — the same rule rephrased as a *verifiable requirement with a
  rationale*: the fix is rejected in review unless the reply quotes runtime
  values observed with the debugger before the first edit, because
  read-only fixes have shipped wrong root causes. Differs from *strong* in
  having a checkable artifact and a reason. (Observed: 100% compliance.)
- **prompt** — the gate requirement placed in the task prompt (user turn)
  instead of `CLAUDE.md`.
- **with / without** (tier 2) — *without*: no debugger at all, the agent
  just reads; *with*: debugger available and required via the gate
  phrasing, since a passive note yields ~0% use and tier 2 measures
  benefit-when-used.

**Measurements:**

- **adoption** — whether a run invoked `gdbg` at least once ("3/5" = 3 of 5
  runs did). `gdbg_calls` counts the invocations.
- **tokens** — the sum of all usage counters reported by the headless run
  (input + output + cache reads/writes across every turn). Comparable
  within one machine and study; host-level config inflates it ~11% (see
  *hermetic*), so cross-machine absolute comparisons are unreliable.
- **passed** — whether `go test` is green after the agent finishes. In tier
  2 the regression test and snapshots are re-checked-out first, so editing
  them cannot fake a pass.
- **baseline red** — sanity check before each tier-2 run: the overlaid
  regression test must FAIL at the rewound commit, or the run is invalid.

**Environments:**

- **hermetic** — the run sees a pristine user config (`CLAUDE_CONFIG_DIR`
  pointed at a fresh directory holding only credentials; `GDBG_HERMETIC`
  in the harness): no global `CLAUDE.md`, personal skills, plugins, MCP
  servers, or session memory from the host machine. Non-hermetic runs
  inherit all of that invisibly, which measurably shifts both tokens and
  adoption — see [FINDINGS.md](FINDINGS.md), "Methodology hazard".
- **workdir isolation** — task copies live under `/tmp`, never inside this
  repo; a workdir inside a repo makes the nested agent resolve that repo as
  its project root and inherit its `CLAUDE.md` and session memory.

**Tiers:**

- **tier 1** (`tasks/`, harness in `adopt/`) — small purpose-built modules
  (20–350 lines) with planted bugs.
- **tier 2** (`repo/`) — real historical bugs in esbuild (~95k lines): the
  worktree is rewound to the commit *before* each fix, only the regression
  test is overlaid, and the agent must re-derive the fix.

## Results

See [FINDINGS.md](FINDINGS.md) for the numbers and how they compare to the
original Rust experiment.
