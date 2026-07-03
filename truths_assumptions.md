# Vision

Turn runtime observation into durable, trainable knowledge, so that agents understand large systems the way no one can by reading alone.

# Problem

## Truths

- Coding agents default to read-and-guess: a passively available debugger gets used in 0% of runs (0/30 controls, two languages).
- Up to at least a 95k-line codebase, reading is sufficient: read-only agents fixed 100% of ~60 benchmark bugs, cheaper on average than debugger runs.
- Read-loop transcripts confabulate: they assert unobserved runtime behavior as fact (~2 fabricated claims per run in the Rust grounding analysis).
- Telling the agent to use the debugger doesn't work reliably: bare mandates score 0–5/5 depending on ambient machine context and programming language.
- Whatever an agent learns at runtime is thrown away when the session ends — there is no persistence mechanism today.
- Written knowledge bases are ignored: agents skip docs even when instructed to read them, mirroring the 0% passive-tool adoption.
- The debugger's cost penalty shrinks monotonically as reading gets harder (1.3× on toy bugs → token parity and faster wall-clock on the hardest real esbuild bugs).

## Assumptions

- Beyond some system size/complexity (~tsz scale, 1.7M+ lines), reading stops working entirely and runtime observation becomes necessary (evidence: one 90%-cheaper anecdote, n=1).
- Enterprise pain concentrates in cross-service integration bugs that neither unit tests (which mock the other services) nor code reading can reach.
- Production traces can't substitute for debugging because they only cover code paths that were actually taken and don't allow exploratory execution.
- The economic buyer cares enough about debugging/verification quality to fund infrastructure work (vs. accepting read-and-guess agents that already pass their tests).

# Solution

## Truths

- Adoption is solvable today without training: a verifiable-artifact gate ("quote observed runtime values before your first edit or the fix is rejected") gets 100% compliance across languages and environments at ~1.5× token cost.
- Debugger trajectories carry ~3× more grounded runtime observations than read-only trajectories (Rust grounding analysis).
- Breakpoint sessions allow counterfactual exploration: live values can be changed to force other branches without restarting (demonstrated via `set … --then continue`).
- Agent-usable debugger tooling is buildable and robust on mainstream stacks (rdbg on lldb, gdbg on Delve — the latter ran flawlessly inside a real 95k-line compiler).

## Assumptions

- Debugger observations are causally load-bearing rather than post-hoc theater — current evidence leans against (6/6 forced episodes show surface grounding but only 1/6 show an observation actually feeding the fix; fix insight came from reading in 8/10 Rust debugger runs).
- Debugger use meaningfully reduces confabulation — so far it barely moves it (2.0 → 1.8 unverified claims per run).
- Runtime facts distilled into a knowledge base will actually be consumed by agents — in tension with the docs-are-ignored truth unless enforced by gates or baked into weights.
- RL/post-training on debugger trajectories produces repo-specialist models that beat frontier models on cost or quality (untested; outcome-only RL is predicted net-negative by the Rust analysis, so it hinges on process rewards that can distinguish causal grounding from theater).
- Multi-service distributed breakpoint debugging (the Airbnb scenario) is buildable and acceptable inside enterprise dev environments (no such tool exists yet).
- A debugger-native model finds more security vulnerabilities than read-only or trace-based approaches (no data yet; inherits the theater risk in amplified form since fabricated evidence arrives pre-credentialed).
- The ~1.5–2× per-session cost of grounded debugging amortizes because the extracted knowledge is reusable across sessions (unproven — persistence and reuse are exactly what doesn't exist today).
