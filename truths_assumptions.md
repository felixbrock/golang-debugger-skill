# Foundation — Truths & Assumptions

The canonical fact base for any reasoning about the vision or short-term MVPs. If a pitch,
MVP idea, or experiment design contradicts a ✅ or ❌ line here, the idea is wrong (or the
line needs new evidence and an edit). This supersedes the earlier truths/assumptions list
that lived in this file: the Go-only items are merged, corrected, and extended with the
Rust-side findings (tsz, uv), the thirdface explorer PoC, and the RL environment work.

> The canonical copy is `mvp-brain-dump/foundation.md`; this is a synced snapshot — edit
> there first.

**Status legend:** ✅ measured truth · ⚠️ partially resolved / pattern-not-law · ❓ untested
assumption · ❌ falsified (do not build on it).

**Provenance tags:** `[Go]` gdbg study — toys, esbuild 95k, Kubernetes 3.6M, cross-service,
~200 runs ([benchmarks/FINDINGS.md](benchmarks/FINDINGS.md)) ·
`[Rust]` tsz 1.7M + uv studies ([02-empirical-findings.md](../mvp-brain-dump/02-empirical-findings.md),
appendix) · `[PoC]` thirdface explorer ([03-explorer-poc.md](../mvp-brain-dump/03-explorer-poc.md)) ·
`[RL]` RustDebugEnv ([06-rl-research.md](../mvp-brain-dump/06-rl-research.md)) · `[Ext]`
externally published result we rely on but did not measure ourselves
([resources.md](../mvp-brain-dump/resources.md)).

## Vision

Turn runtime observation into durable, verified knowledge, so agents understand large
*live* systems the way no one can by reading alone — answering the runtime and
cross-service questions an agent correctly cannot answer from code.

## 1. What agents do by default (adoption)

- ✅ Coding agents default to read-and-guess: a passively available debugger gets used in
  0% of runs (0/30 controls, two languages). `[Go][Rust]`
- ✅ Plain orders ("use the debugger") don't work reliably: 0–5/5 compliance depending on
  ambient machine config, leaked memory, and programming language (Go 1/5 vs Rust 4/5,
  same model, same clean machine). `[Go]`
- ✅ A verifiable proof requirement ("quote observed runtime values before your first edit
  or the fix is rejected", with the reason) gets 100% compliance across languages and
  environments, at ~1.5× token cost. Adoption is solvable today without training. `[Go]`
- ✅ Written knowledge bases are ignored: agents skip docs even when instructed, mirroring
  the 0% passive-tool adoption. Mentioning a strategy in a skill file isn't enough either —
  `trace` was documented with recipes and used once in 159 commands. `[Go]`
- ✅ Adoption is difficulty-gated and agents already have grounding metacognition: unforced,
  capable agents never debug legible bugs (0/7) and *voluntarily* observe non-local ones —
  including 5/5 unprompted live-service probes on the cross-service bug, something that
  never happened in ~200 single-repo runs. `[Go][RL]`

## 2. Where reading suffices — and where it stops

- ✅ Wherever a failing test localizes the bug, reading is sufficient at any repo size:
  read-only agents fixed 100% of ~70 test-given bugs, including 10/10 real Kubernetes bugs
  in 3.6M lines, cheaper on average than debugger runs. `[Go]`
- ✅ Reading cost does not grow with codebase size when a test localizes: the Kubernetes
  read arm (3.6M lines) averaged *half* the tokens of the esbuild read arm (95k) — the
  test collapses the search no matter how much code surrounds it. Repo size is the wrong
  variable. `[Go]`
- ✅ A fix-commit regression test is a post-hoc oracle: it encodes the fixer's finished
  diagnosis, so test-given benchmarks measure *repair*, not *diagnosis*. Removing the tests
  (real bug reports only) exploded read costs ~5× and broke the 100% fix rate for the
  first time. `[Go]`
- ✅ In diagnosis-time conditions correctness finally differentiates, in both directions:
  audited scores read-only 8/9 vs debugger-mandated 5/9 on the same 9 Kubernetes bugs.
  Forced observation changes outcomes — not always for the better. `[Go]`
- ✅ The value variable is **signal-to-cause distance** (how badly the failure localizes),
  not lines of code: package-local test → debugger only adds cost (1.24×–1.88×, 0/10 k8s
  wins); symptom far from cause → reading thrashes (8.8M/22.9M tokens on tsz — whether
  debugger *execution* recoups that cost is the ⚠️ below, and whether distance explains
  both datasets still rests on 3 unreproduced tsz cases: [07](../mvp-brain-dump/07-open-questions.md) #10);
  cause not readable at all → observation is the only move. `[Go][Rust]`
- ⚠️ **Correction to the single-run tsz numbers (−49%/−70%):** the later multi-trial sweep
  on the same closed codebase found *no cell where debugger execution drove a token win* —
  every win came at 0 launches, i.e. from the SKILL's approach-guidance, and cells that
  did launch on hard cases thrashed (+1354%/+1371%). On **closed** code, treat "the
  debugger saves tokens" as unproven; what's real there is prompt-guidance plus
  variance-narrowing (next section). The crossover claim survives only where the deciding
  fact is expensive-to-find *and* not derivable by reading. `[Rust]`
- ✅ On a repo whose bugs are legible (uv, 72 crates), a well-prompted agent correctly
  declines the debugger 11/11 times — the waste-elimination generalizes, so an agent can
  safely *have* the tool everywhere. Bug distribution predicts value, not repo size.
  `[Rust]`
- ✅ No single agent context holds a large system: task reliability collapses as
  environment context grows (Opus 96%→34% success from 8K→128K tokens) and agents burn
  ~56% of turns on exploration. Whole-system knowledge work must be *orchestrated*
  (plan→shard→map→reduce — [04-frameworks.md](../mvp-brain-dump/04-frameworks.md) §5),
  not held in one head. `[Ext: Cognition]`

## 3. What observation actually buys (and costs)

- ✅ Runtime observation can beat a confident wrong prior: the study's single debugger
  correctness win came when the read-only agent pattern-matched the symptom to a
  *different* known bug and argued away the report, while the debugger arm observed the
  real values and fixed the right thing. This is the exact failure mode observation is for
  — and reading never questions it. `[Go]`
- ⚠️ Mandated observation carries a repair risk — **evidence anchoring**: all three genuine
  debugger-arm failures fixed exactly the code path the agent had watched (two with
  correct, evidence-backed diagnoses), missing the invariant or breaking a sibling path.
  Runtime facts are locally true and globally incomplete. (n=3 — pattern, not law;
  replication + mitigation test: [07](../mvp-brain-dump/07-open-questions.md) #9.) `[Go]`
- ✅ Debugger runs contain more real evidence: 2.6–3.2× more observed runtime facts per run
  than read-only runs. `[Go][Rust]`
- ✅ But most forced observation is theater — the agent decides by reading, then debugs for
  show: 75–93% of forced sessions had no observation that drove the fix. The genuine cases
  concentrate exactly where complexity is highest. `[Go][Rust][RL]`
- ✅ Distinguishing genuine observation from theater is automatable at judge-model level
  (`grounding/judge.py` reproduces the causal/surface split across both studies) — but the
  *causal* label is sparse (fires ~1/6), which is the central unsolved problem for
  training on it. `[Go][RL]`
- ✅ The winning mechanism is **"tap, don't walk"**: winning runs averaged ~4 debugger
  calls (break at the sink, read which path fired / which breakpoint *never* fired,
  backtrace, then go read the deciding code); losing runs averaged ~45 (stepping,
  eval-loops, print-instrumentation). The debugger aims your reading; it rarely hands you
  the fix. `[Rust]`
- ✅ Telemetry agrees: agents use a debugger as a *value oracle at a chosen line* —
  targeted `eval` (45–60% of all commands) plus conditional breakpoints — and skip the
  human surface entirely (zero/near-zero stepping, stack-walking, state mutation,
  goroutine inspection). Build for the oracle, not the IDE debugger. `[Go]`
- ✅ The one clean debugger-execution value on closed code is **variance-narrowing**: it
  makes a weak model more *consistent* (Sonnet ×10.8→×5.5; codex-low ×29.4→×1.7),
  confirmed across two vendors. `[Rust]`
- ✅ Loud tool failure is load-bearing, not cosmetic: the tsz win only existed after
  breakpoints that never fired started reporting "NOT BOUND"; silent failures had made the
  debugger arm *more* expensive. Worse than the cost: a silently-unbound breakpoint
  **manufactures false evidence** ("this function is never called") that the agent then
  trusts — bad tool output doesn't just waste tokens, it lies. Boring reliability — tools
  that never lie silently — is part of the moat. `[Rust][Go]`
- ✅ Breakpoint sessions allow counterfactual exploration (change a live value, force the
  other branch, no restart) — demonstrated, though agents almost never reach for it
  unprompted (zero `set` calls in all telemetry). `[Go]`
- ✅ Agent-usable debugger tooling is buildable and robust on mainstream stacks (gdbg ran
  flawlessly inside a real 95k-line compiler and 3.6M-line Kubernetes). `[Go]`

## 4. Facts not in code — the cross-service / open-system layer

- ✅ When the truth isn't in any readable file (cross-service contract bug, other service's
  source unavailable), agents switch to runtime observation *on their own* — 5/5 read-arm
  runs probed the live service unprompted. What fails isn't the agent; it's the assumption
  that it will guess. `[Go]`
- ✅ For contract bugs the cheapest correct observation is the **service boundary**, not
  in-process debugging: the program had already discarded the evidence (decoded the JSON
  into a struct without the deciding field) before a debugger could see it. One curl beat
  nine debugger calls; even debugger-mandated runs fell back to probing the wire. The
  primitive is the boundary tap — tracer/interceptor/contract-differ — with the in-process
  debugger as a complement. `[Go]`
- ✅ An explorer agent, running unattended on a real multi-service system, can autonomously
  extract *verified* facts-not-in-code with provenance and second-surface verification:
  a live per-tenant flag override producing different workflow inputs from identical code,
  and wire-vs-declared-schema contract drift. Feasibility is proven. `[PoC]`
- ✅ A careful strong model doesn't confabulate about runtime — it **hedges correctly**
  ("depends on runtime, I can't know from code") and even flags contract drift itself from
  source. Unverified-claim rates are low (0.2–0.7/run) debugger or not. So the durable
  value is **resolving the correct hedge into a concrete verified answer**, not catching a
  liar; the "confabulation firewall" framing is retired. `[Go][PoC]`
- ⚠️ Nuance from the RL side: agents *assert* ungrounded runtime claims at a high rate
  (0.58–0.77) with zero "let me check first" abstention — they're just rarely *wrong*.
  The missing check-first reflex is a training target even though wrongness isn't. `[RL]`
- ✅ Facts-not-in-code taxonomy (value rises down the list; closed repos only have class 1):
  (1) latent-in-code, (2) input/data-dependent state, (3) config/flag/tenant splits,
  (4) cross-service wire contracts, (5) emergent/temporal (which provider served, fallback
  fired, retries, races). Classes 3–5 exist only in open systems and are the product.
  `[Rust][PoC]` — see [04-frameworks.md](../mvp-brain-dump/04-frameworks.md).
- ✅ Repro is the real cost: even on a system we built, the stack needed fixing (stale
  container, flag-relay bug) before anything could be observed. "Make the customer's
  system observable/runnable" is itself the enterprise dirty-work — both the moat and the
  load-bearing GTM risk. `[PoC]`

## 5. Persistence, training, RL

- ✅ Whatever an agent learns at runtime is thrown away when the session ends — no
  persistence mechanism exists today. `[Go]`
- ✅ Contrastive training pairs exist naturally: per bug, two trajectories with the *same*
  fix but different epistemics (guessed vs grounded) — DPO-ready observe-before-assert
  data. `[RL]`
- ❓ Runtime facts distilled into a knowledge base will actually be consumed by agents — in
  tension with the docs-are-ignored truth unless enforced by gates or baked into weights.
  Working hypothesis: hybrid — train on the *stable* reality (topology, contract shapes,
  edge-case classes), live tool for current specifics; weights alone go stale weekly. `[Go]`
- ❓ RL/post-training on causally-filtered debugger trajectories produces repo-specialist
  models that beat frontier on cost or quality. Untested, cheap to test, and adoption
  doesn't need it (the proof gate already gets 100%) — keep the model story secondary
  until proven. `[RL]`
- ❓ The fused training design (predict-then-verify grading inside a real fix task: terminal
  fix reward + dense per-step calibration term) kills the theater problem. Designed, not
  run. `[RL]`
- ❓ The **world-model framing**: a model trained on causal (state, action, outcome)
  relations from code + live usage predicts *what the system does next* — unifying the
  explorer (data collector), predict-then-verify (training objective), the grounding judge
  (causal filter), and the weights-vs-tool question. Must beat "just ask the live system"
  on coverage/latency/availability, and survive staleness. See
  [08-world-model.md](../mvp-brain-dump/08-world-model.md).
- ❓ The ~1.5–2× per-session cost of grounded debugging amortizes because extracted
  knowledge is reusable across sessions — unproven; persistence and reuse are exactly what
  doesn't exist today. Test: [07](../mvp-brain-dump/07-open-questions.md) #8. `[Go]`

## 6. Market & GTM

- ❓ Enterprise pain concentrates in cross-service integration bugs that neither unit tests
  (which mock the other services) nor code reading can reach — plausible, untested; in our
  2-service toy both arms fixed the bug 5/5. This is the market bet. `[Go]`
- ❓ At real system scale (hundreds of services), locating *which* boundary carries the bad
  data is the hard part. Our 2-service result (found with one curl) says nothing about
  N-service localization — which is where boundary-observation orchestration would earn
  its keep. `[Go]`
- ❓ Production traces can't substitute for exploration: they only cover paths actually
  taken and allow no counterfactuals. Plausible, unmeasured — and it is the moat claim
  against every observability incumbent. Test: [07](../mvp-brain-dump/07-open-questions.md) #7. `[Go]`
- ❓ The economic buyer cares enough about diagnosis/verification quality to fund
  infrastructure work, vs. accepting read-and-guess agents that already pass their tests.
  Sharpened by the k8s finding: a well-tested repo localizes most regressions for free —
  the sell is the *badly-localized* residue (integration failures, emergent behavior,
  wrong-output bugs), which is exactly what stays painful as models improve. Validate
  through the design-partner conversations: [07](../mvp-brain-dump/07-open-questions.md) #14. `[Go]`
- ❓ A debugger-native/grounded explorer finds more security vulnerabilities than read-only
  or trace-based approaches — no data; inherits the theater risk in amplified form
  (fabricated evidence arrives pre-credentialed). Expansion, not wedge. `[Go]`
- ❓ Warn-not-block is the viable pre-merge shape (a gate that false-positives gets
  disabled). Untested. `[Rust]`
- ❓ Single-process debugging gets commoditized by the frontier labs soon — so it can only
  ever be the on-ramp, never the product. Consistent with everything measured (the value
  was never the debugger); treat any debugger-shaped MVP as a wedge with a countdown.
- ❓ On-prem/privacy only differentiates at **whole-agent-stack** level: one private probing
  agent among coding agents that ship context to frontier APIs satisfies no requirement.
  Moat for the air-gapped/regulated segment; table stakes elsewhere. Validate:
  [07](../mvp-brain-dump/07-open-questions.md) #14.
- ❓ European enterprise buyers may refuse Chinese open-source base models on compliance
  grounds — a constraint on the cheap-specialist-model path (GLM et al.).

## 7. Falsified — do not build on these

- ❌ "The debugger improves fix rate on test-given bugs." Never, anywhere: not at 20 lines,
  95k, 1.7M, or 3.6M. With a test, it changes cost only. `[Go][Rust]`
- ❌ "Repo size drives debugger value." Kubernetes at 3.6M lines: 0/10 wins, and reading
  got *cheaper* than on the 38×-smaller repo. `[Go]`
- ❌ "On closed code, debugger execution saves tokens." Zero cells across the multi-model /
  effort matrix; wins were prompt-guidance at 0 launches. `[Rust]`
- ❌ "An unfiltered harvest of debugger sessions is useful training data." 75–93% theater;
  the pitch stands only with a causal-grounding filter in the loop. `[Go][Rust][RL]`
- ❌ "Debugger use meaningfully reduces confabulation." It barely moves it (0.2 vs 0.2
  claims/run), and there is little confabulation left to remove. `[Go]`
- ❌ "We stop your agent from hallucinating about your system" (the confabulation-firewall
  pitch). Strong models hedge; the pitch is resolving the hedge. `[Go][PoC]`
- ❌ "Telling the agent to use the tool is enough." Plain orders swing 0–5/5 with ambient
  config and language; only the checkable proof requirement is stable. `[Go]`
- ❌ "Hidden upstream regression tests are a fair correctness oracle." 3 of 7 symptom-arm
  "failures" were verification artifacts (the test demanded upstream's own API shape).
  Hidden-test fix rates are floors until a transcript audit. `[Go]`

## 8. Method rules (how we keep the evidence honest)

- Isolate the config and the workdir or your own settings and memory become invisible
  variables: leaked session memory both caused failures and suppressed tool use; host
  config added ~11% tokens to every run. Hermetic single-commit checkouts, post-cutoff
  cases, no web. `[Go][Rust]`
- Single runs vary ±30%; quote multi-trial medians for fine claims, trust only aggregate
  direction from one-run-per-cell grids. `[Go][Rust]`
- Transcript-level tool-call counting undercounts real usage 2–2.6× (agents chain commands
  per shell line) — instrument the tool daemon, not the transcript. `[Go]`
- Audit failed runs against the real fix before believing a fix-rate delta; score "did it
  fix the reported bug," not "did it match upstream's diff." `[Go]`

## 9. The load-bearing open bets, in order

1. ❓ Value survives on a real N-service system with a genuinely data-/config-/wiring-
   dependent bug — base agent ships it, grounded agent catches it. The single experiment
   that turns the story into a company. ([07-open-questions.md](../mvp-brain-dump/07-open-questions.md) #1)
2. ❓ N-service boundary *localization* (which wire carries the bad data) is where
   orchestration earns its keep — nothing measured beyond 2 services.
3. ❓ The data-opacity correctness anchor: bugs where the code *looks* correct and reading
   actively misleads. The kube-reserved wrong-prior win (one case) is the existence proof;
   nobody has built the benchmark.
4. ❓ Specialist small model beats frontier on one repo (cheap to test; keep secondary).
5. ❓ Repro/observability-bootstrap on a system we *don't* own (the GTM crux).
