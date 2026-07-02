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

## Results

See [FINDINGS.md](FINDINGS.md) for the numbers and how they compare to the
original Rust experiment.
