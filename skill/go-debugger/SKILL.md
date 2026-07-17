---
name: go-debugger
description: Debug a Go program or failing test with gdbg — set breakpoints (line, function, conditional, hit-count, logpoint, or watchpoint), run and step, read locals as real Go values (slices/maps/structs/interfaces), change a variable mid-run, inspect goroutines, and find declarations/definitions/references. Reach for it on a *runtime* question — a wrong value, an unexpected branch/state, a panic, a goroutine doing something unexplained — when the symptom is far from its cause and reading has stalled, instead of adding fmt.Println. Read first when the failing test plus a quick read already point at the fix; the debugger earns its cost where the flow is too tangled or too distant to trace by eye.
---

# go-debugger

Debug from the command line with `gdbg`. It holds one paused process per
project, so breakpoints and state carry across calls. Run `gdbg` with no
arguments for the full command list.

Requires `gdbg` and `dlv` on PATH
(`go install github.com/go-delve/delve/cmd/dlv@latest`); `gopls` for
def/hover/refs. dlv needs a Go toolchain at least as new as it was built with.

## When to reach for it (and when not)

Read first. The debugger earns its cost only on a question you can't answer by
reading — what decides that is not repo size but the **distance between the
symptom and its cause**. Decide *before* you launch:

**Reach for gdbg** when you have a **runtime question at a place you can name**:
- a value is **wrong** and the code that produced it is far from where it
  surfaces — you need the real inputs/flow, not another file read;
- an **unexpected branch, type, or state** at runtime that reading can't pin
  down;
- a **panic** — `launch` then `continue` lands on the raising frame with its
  arguments;
- behavior reading **can't see at all**: what actually crossed a process or
  service boundary, what a goroutine is blocked on;
- you want to **test a fix live** with `set --then continue` before editing.

**Don't launch — just read/grep — when** the failing test plus a quick read
already point at the fix (small, localized bugs): a debugging detour only adds
cost, and it stays pure overhead no matter how big the repo is.

**Stay cheap.** Keep launches few: one session with several breakpoints, or
one `trace`, beats re-launching. If 2–3 probes haven't localized it, stop and
go back to reading — you're probably at the wrong layer, and more debugging
will only burn tokens.

**Fix once, don't churn.** When you've found the cause, make **one** careful,
minimal edit and run the narrowed test. If it still fails, do **not**
guess-and-edit again. Read the exact failing assertion, or break at it and
`vars`/`eval` the actual-vs-expected values to see *why* before you touch the
code again. Better still, validate a fix hypothesis **without editing at
all**: `set` the suspect value and `continue` to watch the outcome change.
More than ~2–3 edit→test cycles means you're guessing — go back to
understanding.

## Tap, don't walk (hard rules)

The debugger **aims your reading**; it rarely hands you the fix. The pattern
that works: break at the **sink** — where the wrong result surfaces (the emit,
the return, the failing assert) — read **which path fired and the deciding
values there**, `bt` back to the code that decided it, and **read that code**.
One or two launches, then you're reading.

- **`bt` named a `file:line` → STOP; do not launch again.** Read that line.
  The fix is in that frame or its **caller** — never break into a *callee* the
  backtrace names just to "confirm" its return value; read the caller that
  decided to use it.
- **Budget: 2 launches (a `trace` counts).** Before a 3rd probe, state in one
  sentence the exact runtime fact you still lack *and cannot get by reading*.
  If you can't, you're done debugging — read.
- **NEVER HIT means the sink is elsewhere: READ, don't re-guess.** Grep the
  wrong value / read the emit site to find the real `file:line`; do not
  relaunch at another guessed location. Never use the debugger to *search* for
  where to break — a failing test that names the wrong value (an error code, a
  message) is a grep task, not a debugger task.
- **`eval` can't call your functions or methods.** Break *inside* the method
  and read its inputs, or read the code. **Never add
  `fmt.Println`/`log.Printf`** — print-debugging round-trips an edit + rerun
  per question and is the signature of losing runs. Use `gdbg vars` to see
  *all* locals at one stop instead of re-launching to change `--capture`.
- **State only what the output shows.** Cite the exact command and the output
  line that proves a claim, or say "unknown". A test that just failed without
  ever pausing does **not** mean your line never ran — check the NEVER HIT
  report on exit, or `gdbg breaks` for hit counts, before trusting silence.
- **Degraded tooling → stop and read.** Breakpoints that won't resolve or
  values that won't `eval` mean the debugger can't answer on this build;
  relaunching won't change that. Stepping is for a short hop you can't read —
  never to traverse a path.

## Start a session

```
gdbg where parseConfig                             # find where to break
gdbg launch --pkg . --break main.go:88 -- --threads 4
gdbg launch --test ./pkg --break pkg/thing.go:120 -- -run TestFailing
gdbg launch --bin-path ./bin/app --break main.go:11    # pre-built binary
```

`--pkg`/`--test` build with the right debug flags automatically. Unrecovered
panics and fatal errors (deadlock, nil map write, …) always stop the program —
no flag needed: `launch` then `continue` lands on the panic with its message,
and auto-selects the first non-runtime frame so `vars` shows the arguments
that caused it.

To watch a value evolve without stepping, `trace` instead of `launch` — it
runs through every hit and returns a table in one call:

```
gdbg trace --pkg . --break main.go:42 --capture i,sum --max 30
gdbg trace --test . --break-fn pkg.emitError --capture msg --bt 5 -- -run TestX
```

Same `--test`/`--pkg` flags as `launch`; `--break-fn` traps a function,
`--bt N` prints the caller chain at every hit. If you catch yourself
repeating `eval` + `continue` at one breakpoint, that whole loop is a single
`trace`.

## Breakpoints

Set or change these any time, including while paused.

```
gdbg break main.go:42                    # line
gdbg break main.go:42 --if "i == 5"      # conditional (any Go expression)
gdbg break main.go:42 --hit 3            # on the 3rd hit ("% 2" = every 2nd)
gdbg break main.go:42 --log "i={i}"      # logpoint (print, don't stop)
gdbg break --fn mypkg.doThing            # entering a function
gdbg watch sum                           # stop the moment sum changes (in scope)
gdbg breaks                              # list with ids; break-rm/break-on/break-off <id>
```

## Run and step

```
gdbg continue                  # to next stop; auto-interrupts after 25s
gdbg step over | in | out | insn         # `out` shows return values
gdbg until main.go:99          # run to a line
gdbg pause                     # interrupt a running program
gdbg restart [--rebuild]       # relaunch; --rebuild picks up code edits
```

## Read and change state

```
gdbg vars                      # args + locals as real Go values
gdbg eval items[0].Qty len(items) sum > 10   # any Go expressions
gdbg set cfg.Threads = 8 --then continue     # change a value, keep running
gdbg watch-expr add total      # re-shown at every stop
gdbg bt                        # backtrace
gdbg list                      # source around the current line
gdbg state                     # stop + locals + watches together
gdbg output                    # the program's stdout/stderr so far
```

Every stop shows only the locals that *changed* (`~ sum = 6 (was 3)`); `vars`
shows everything.

## Goroutines and frames

```
gdbg goroutines                # list with user-code locations
gdbg goroutine <id>            # vars/eval/bt follow the selected goroutine
gdbg frame <n> | up | down     # and the selected frame
```

## Navigate (no session needed)

```
gdbg where <Name>              # declarations of a func/method/type/const/var
gdbg def | hover | refs <file> <line> <col>    # via gopls
```

Several commands in one call: `gdbg do "vars; step over; vars"`.
`gdbg stop` ends the session; `gdbg down` stops the daemon.

## Common loops

- **Wrong or extra output (sink trap).** Trace the function that *emits* the
  wrong artifact with `--bt`: `gdbg trace --test . --break-fn pkg.emit
  --capture <args> --bt 5 -- -run TestX` — each hit shows the value and the
  caller chain that decided it; fix the deciding caller, not the sink.
- **Missing output.** Trap the sink the same way: if gdbg reports the
  breakpoint was NEVER HIT, the decision happened upstream — breakpoint the
  guards that should have called it and read their inputs.
- **Wrong value.** Break where it is computed, `vars`/`eval` to see the real
  inputs, `set` to test a fix without recompiling. Reach for `step` only for a
  short hop you can't read — not to walk the whole path.
- **Panic.** `launch`, `continue` — you land on the raising frame with its
  arguments. `bt`, `frame up` to walk out to your code.
- **Unexpected mutation.** Step until the variable is in scope, `gdbg watch
  <var>`, then `continue` — it stops on the exact line that wrote it.
- **Failing test.** `launch --test ./pkg -- -run TestName`, break at the
  assertion or in the code under test.
- **Hung program.** `continue` (auto-interrupts), or `pause`, then
  `goroutines` to see where everything is blocked.

## Notes

- `eval` takes real Go expressions (indexing, fields, len/cap, comparisons,
  most arithmetic); `set` takes numeric, bool and pointer values.
- Watchpoints are scope-bound: set them where the variable is live; the stop
  reports when one goes out of scope.
- When the program exits, gdbg lists any breakpoints that were NEVER HIT —
  treat that as "wrong location or condition", not as proof the code is
  unreached.
- One paused process per project; a new `launch` replaces it. The daemon
  shuts down after 30 minutes idle. State lives in `.gdbg/` (gitignore it).
