# Vision

Turn runtime observation into durable, trainable knowledge, so that agents understand large systems the way no one can by reading alone.

# Problem

## Truths

- Coding agents default to read-and-guess: a passively available debugger gets used in 0% of runs (0/30 controls, two languages).
- Up to at least a 95k-line codebase, reading is sufficient: read-only agents fixed 100% of ~60 benchmark bugs, cheaper on average than debugger runs.
- Telling the agent to use the debugger doesn't work reliably: plain orders score 0–5/5 depending on ambient machine context and programming language.
- Whatever an agent learns at runtime is thrown away when the session ends — there is no persistence mechanism today.
- Written knowledge bases are ignored: agents skip docs even when instructed to read them, mirroring the 0% passive-tool adoption.
- The debugger's cost penalty shrinks monotonically as reading gets harder (1.3× on toy bugs → token parity and faster wall-clock on the hardest real esbuild bugs).
- Current-generation models barely confabulate runtime behavior on these tasks (0.2–0.7 unverified claims/run, debugger or not) — the "models hallucinate what code does" argument is weakening with model progress.
- When the truth isn't in any readable file (a cross-service contract bug, other service's source unavailable), agents switch to runtime observation on their own — 5/5 read-arm runs probed the live service unprompted, something that never happened in ~200 single-repo runs.
- For contract bugs the cheapest correct observation is the service boundary (one curl showed the hidden field), not in-process debugging — the program had already discarded the evidence before a debugger could see it, and even debugger-mandated runs fell back to probing the wire.

## Assumptions

- Beyond some system size/complexity (~tsz scale, 1.7M+ lines), reading stops working entirely and runtime observation becomes necessary (evidence: one 90%-cheaper anecdote, n=1).
- Enterprise pain concentrates in cross-service integration bugs that neither unit tests (which mock the other services) nor code reading can reach — plausible but untested; in our 2-service toy both arms fixed the bug 5/5.
- Production traces can't substitute for debugging because they only cover code paths that were actually taken and don't allow exploratory execution.
- The economic buyer cares enough about debugging/verification quality to fund infrastructure work (vs. accepting read-and-guess agents that already pass their tests).

# Solution

## Truths

- Adoption is solvable today without training: a verifiable proof requirement ("quote observed runtime values before your first edit or the fix is rejected") gets 100% compliance across languages and environments at ~1.5× token cost.
- Debugger runs contain more real evidence: 2.6× more observed runtime facts than read-only runs on real esbuild bugs (6.7 vs 2.6 per run), replicating the Rust study's 3.2×.
- Most forced debugger use is theater — the agent decides by reading, then debugs for show: only 1/15 toy runs and 3/12 real-bug runs had an observation that actually drove the fix.
- The genuine cases concentrate exactly where complexity is highest: the causal fraction is ~4× higher on real bugs than on toys, and the three genuine cases were the hardest bugs.
- Distinguishing genuine observation from theater is automatable: a judge-model filter (grounding/judge.py) reproduces the Rust project's causal/surface split on our data.
- Breakpoint sessions allow counterfactual exploration: live values can be changed to force other branches without restarting (demonstrated via `set … --then continue`).
- Agent-usable debugger tooling is buildable and robust on mainstream stacks (gdbg ran flawlessly inside a real 95k-line compiler).

## Assumptions

- An unfiltered harvest of debugger sessions is useful training data — measured to be false as stated (75–93% theater); the pitch stands only with a causal-grounding filter in the loop.
- Debugger use meaningfully reduces confabulation — measured: it barely moves it (0.2 vs 0.2 claims/run on real bugs), and there is little confabulation left to remove.
- Runtime facts distilled into a knowledge base will actually be consumed by agents — in tension with the docs-are-ignored truth unless enforced by gates or baked into weights.
- RL/post-training on (filtered) debugger trajectories produces repo-specialist models that beat frontier models on cost or quality (untested).
- Multi-service distributed breakpoint debugging (the Airbnb scenario) is buildable and acceptable inside enterprise dev environments — no such tool exists yet; this orchestration layer, not the single-repo debugger, is where the structural value would live.
- At real system scale (hundreds of services), locating WHICH boundary carries the bad data is the hard part — our 2-service result (agents find it with one curl) says nothing about N-service localization, which is where boundary-observation orchestration would earn its keep.
- A debugger-native model finds more security vulnerabilities than read-only or trace-based approaches (no data; inherits the theater risk in amplified form since fabricated evidence arrives pre-credentialed).
- The ~1.5–2× per-session cost of grounded debugging amortizes because the extracted knowledge is reusable across sessions (unproven — persistence and reuse are exactly what doesn't exist today).
