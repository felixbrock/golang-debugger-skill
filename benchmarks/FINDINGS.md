# What the benchmarks showed

We gave Claude (Opus 4.8, via Claude Code) a Go project with a bug and a
failing test, said "fix it", and repeated that many times under different
conditions — measuring whether it used the debugger, what the run cost in
tokens, and whether the test passed afterward. This replicates the
[rust-debugger-skill](https://github.com/mohsen1/rust-debugger-skill)
experiments (which used the same model) and then extends them. Plain-language
definitions of every term are in [README.md](README.md#terminology).

Roughly 200 runs total, across ~2026-07-02/03, with gdbg + Delve 1.27 on
Go 1.26.

## The headline result

**The agent never needed the debugger.** It fixed every bug in every
condition — small planted bugs, hard multi-file bugs, and 12 real historical
esbuild bugs — by reading the code. Using the debugger never improved
correctness and always cost more tokens (1.3–1.8× on clean measurements).
The extra cost shrinks as bugs get harder to read, reaching roughly
break-even on the hardest real bugs, but it never became a saving.

## Both experiments, side by side

Small planted-bug tasks, 5 runs per condition. "Mandate" is a CLAUDE.md
ordering the agent to use the debugger; "note" just mentions it exists.
Go numbers come from clean runs (fresh config, isolated work folders — see
the contamination section for why that matters).

- **accumulator (easy bug, visible in the source)**
  - Go: mandate followed 1/5, note 0/5 — debugger runs 142k tokens vs 108k
    read-only (reading 1.31× cheaper); all passed.
  - Rust: mandate followed 5/5, note 0/5 — 386k vs 135k (reading 2.85×
    cheaper); all passed.
- **rpncalc (bug invisible in the output — swapped operands)**
  - Go: mandate followed 3/5, note 0/5 — 191k vs 128k (reading 1.49×
    cheaper); all passed.
  - Rust: mandate followed 4/5, note 0/5 — 278k vs 153k (reading 1.82×
    cheaper); all passed.

## What replicated from the Rust study

- **Just making the debugger available does nothing.** With only a "this
  exists" note, the agent used it in 0 of 20 Go runs and 0 of 10 Rust runs.
- **Using the debugger never helped correctness.** Every run passed in both
  studies; forcing the debugger only added cost.

## Orders don't work reliably — asking for proof does

The bossy CLAUDE.md that got 5/5 compliance in the Rust study only got 3/5
here at first, and probing why became the most useful part of the study.
Both studies used the same model, so we tested everything else. A follow-up
on the easy bug, 5 runs per condition:

- **strong** (an order: "MANDATORY — use the debugger before editing"):
  followed 3/5, 158k mean tokens. The agent often decides "the bug is clear
  from the source" and just ignores the order.
- **gate** (a checkable requirement with a reason: "your fix is rejected
  unless you quote runtime values you actually observed before your first
  edit, because read-only fixes have shipped wrong root causes"):
  followed **5/5**, 249k.
- **prompt** (the same requirement written into the task message instead of
  CLAUDE.md): followed **5/5**, 284k.
- **control** (a one-line note): 0/5, 120k.

The difference between "strong" and "gate" is that the gate demands something
checkable — quoted evidence — and explains why. Orders without a check get
second-guessed; proof requirements don't.

### The environment quietly changes the results

We then discovered that every benchmark run had been silently inheriting
things from the host machine: the user's global Claude config, personal
skills, plugins — and, because work folders originally lived inside this
repo, even this project's session memory. Re-running with a completely fresh
config ("clean" below) against the earlier runs:

- note: 0/5 clean, 0/5 with host config — stable.
- order (strong): **1/5 clean**, 3/5 with host config, 0/5 when stale memory
  leaked in — unstable, swung by ambient context.
- proof requirement (gate): **5/5 clean and 5/5 with host config** — stable,
  and cheapest when clean (165k, only 1.53× the read-only baseline).
- The host config also added ~11% tokens to every single run (its skills and
  plugins ride along in the system prompt).
- The Rust study's 5/5 for the order ran on its author's machine with its
  own uncontrolled config — best read as another draw from a noisy setup.

### The language matters too

To separate "language" from "machine", we ran the Rust study's own harness
and task on our machine with the fresh config (Rust toolchain + lldb
installed locally, rdbg built from their repo):

- Go order, clean, this machine: followed 1/5 (142k tokens).
- Rust order, clean, this machine: followed **4/5** (187k).
- Rust order, their machine, their config: 5/5 (386k).
- Notes: 0/5 everywhere; every run passed everywhere.

Same model, same machine, same clean config, same prompt structure,
near-identical bug — and the agent obeys the order 4× more often in Rust.
The likely reason: the model trusts its ability to read Go more, so it skips
the debugger more. (Cells are only 5 runs, so treat this as a strong hint,
not proof.) The clean Rust runs also show what the author's machine had
added: 2–3 debugger calls at ~187k tokens clean, versus 6–9 calls at ~386k
in the original — ambient context roughly doubled the cost of debugging.

So compliance with a plain order depends on the instructions, the ambient
environment, *and* the language. Only the proof requirement was immune to
all three.

## Warning for anyone benchmarking agents this way

Nested `claude -p` runs inherit whatever the host machine has configured. In
the first pass of this experiment (work folders inside the repo), leaked
session memory both *caused* failures (an agent exported a stale toolchain
setting it read from memory, and the debugger refused to start — two runs
burned 396k/439k tokens recovering) and *suppressed* debugger use elsewhere
(a memory describing debugger failures scared agents off: 0/5 vs 3/5 clean).
The harness now keeps work folders under `/tmp` and supports a fresh config
via `GDBG_HERMETIC`; contaminated rows are archived in
`results-contaminated.jsonl`. If you benchmark agents with Claude Code:
isolate the work folders and the config, or your own settings and memory
become invisible variables.

One real fix came out of this: the gdbg daemon used to keep the environment
of whichever client started it, so one bad environment poisoned every later
launch. `launch`/`trace` now use the requesting client's environment.

## Harder bugs: can the debugger win?

All the cells above passed everything, so they only measured cost. We built
two tasks designed so reading should struggle, and ran read-only vs
debugger-required, 5 runs each (clean config):

- **pipeline** — a 6-stage log pipeline (~350 lines, 8 files); the bug is
  one reversed comparison at a window boundary, and the test fails on an
  end-of-pipeline number that points at nothing.
  - read-only: **5/5 passed**, 162k tokens, 57s.
  - debugger required: 5/5 passed, 297k (1.83×), 90s.
- **panic_deep** — a module that crashes two files away from the actual
  mistake (an index recorded off by one at build time).
  - read-only: **5/5 passed**, 136k tokens, 28s.
  - debugger required: 5/5 passed, 241k (1.77×), 54s.

Reading did not crack. The read-only transcripts all look the same: read the
eight files once, edit the buggy line, run the test — no print statements,
no second attempt. At this size (up to ~350 lines) there is no bug subtle
enough to make the debugger pay off for this model.

## Real bugs in a real codebase (esbuild, 95k lines)

The last place a saving was plausible: bugs whose codebase is too large to
just read. We mined 12 real, merged esbuild bug fixes (each ships a
regression test; each was mechanically checked to fail before the fix and
pass with it), rewound the repo to just before each fix, and asked the agent
to rediscover it — with and without the debugger. Tampering with the test
can't fake a pass (the test files are restored before checking).

- read-only: **12/12 solved**, 708k mean tokens (419k median), 173s mean.
- debugger required: **12/12 solved**, 1,280k mean (1.8×), 707k median, 227s.

Still no crossing — but the gap closed at the hard end:

- On the two bugs where *reading itself* was most expensive (~2M tokens),
  the debugger arm matched the token cost and was clearly faster (280s vs
  472s wall time on one; 213s vs 270s on the other).
- The debugger-arm average is dragged up by one 6.2M-token run — not tool
  failure (1 error in 24 calls) but a genuinely long investigation with
  conditional breakpoints deep in the renamer. Excluding that case from both
  arms: 608k vs 829k (1.36×).
- gdbg held up operationally in a real compiler codebase: breakpoints in
  deep internals, expressions over real data structures, almost no failed
  commands.

## Is debugger data *better* data? (the grounding analysis)

If the debugger doesn't make agents more correct or cheaper, its remaining
value is that debugger runs contain *observed facts* instead of guesses —
potentially useful as evidence, as a knowledge base, or as training data. We
had a judge model score all 49 transcripts: how many runtime facts were
actually observed, how many runtime claims were asserted without evidence,
and — the key question — whether any debugger observation actually *changed
or determined* the diagnosis, versus the agent deciding by reading and then
running the debugger for show (we call that "theater": the answer was
written down first, the working-out performed afterwards).

- **Debugger runs do contain more real observations.** On real esbuild
  bugs: 6.7 observed facts per run (4.8 from the debugger) vs 2.6 for
  read-only runs — 2.6× more. Matches the Rust study's 3.2×.
- **But most debugger use is theater.** On the small tasks, only 1 of 15
  forced-debugger runs had an observation that actually drove the fix (93%
  theater) — almost exactly the Rust project's own measurement (1/6). On
  real esbuild bugs it improves to 3 of 12 (75% theater).
- **The genuine cases cluster at the hard end.** The three real-bug runs
  where observation drove the diagnosis were among the hardest — including
  the 6.2M-token investigation, where seeing an unexpected value redirected
  a wrong diagnosis. Difficulty makes debugger evidence real.
- **"Models hallucinate runtime behavior" is fading as an argument.** This
  model asserts very few unverified runtime claims anywhere (0.2–0.7 per
  run), debugger or not. The Rust study's ~2 fabricated claims per run looks
  partly like an older-model problem.
- Failed debugger commands rise with realism: 2.2 per run in esbuild vs 0.6
  on toys.

Implication for "harvest debugger sessions as training data": getting agents
to *use* the debugger is trivial (the proof requirement gets 100%), but an
unfiltered harvest would be 75–93% theater. The hard and valuable part is
detecting whether an observation actually informed the conclusion —
`grounding/judge.py` is a first prototype of that filter.

## Bottom line

1. A passive "this tool exists" note gets ~0% use, in every language and
   environment tested.
2. Compliance with a plain order is unreliable — it swings with the ambient
   machine setup and with the programming language. A checkable proof
   requirement ("quote what you observed") gets 100% compliance everywhere,
   at ~1.5× cost.
3. For fixing bugs, the debugger never beat reading at any scale we could
   test (up to a real 95k-line codebase) — costs converge toward break-even
   on the hardest bugs but never cross.
4. The debugger's real, measured value is evidence quality: runs contain
   ~3× more observed fact. But most forced observation is decorative; on
   real bugs only ~25% of sessions produced an observation that genuinely
   drove the fix. Anything built on debugger data — verification gates,
   knowledge bases, RL training — stands or falls on filtering the genuine
   sessions from the theater.
