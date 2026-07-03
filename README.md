# `gdbg`

**Debug a Go program from a coding agent — set breakpoints, run, and read the
actual values of variables instead of guessing from `fmt.Println`.** A single
binary that wraps [Delve](https://github.com/go-delve/delve), usable as a CLI
skill or an MCP server for Claude Code and Codex.

The Go sibling of [rust-debugger-skill](https://github.com/mohsen1/rust-debugger-skill).

```console
$ gdbg launch --pkg examples/demo --break main.go:18
>>> STOP [breakpoint] main.total  main.go:18  (goroutine 1)
   ->    18 |         sum += it.Qty
+ items = [main.Item{Name: "apple", Qty: 3}, main.Item{Name: "pear", Qty: 0}, …
+ sum = 0
$ gdbg vars
  items: []main.Item (len 3)
    [0]: main.Item
      Name: string = "apple"
      Qty: int = 3
  sum: int = 0
$ gdbg eval items[0].Qty      # int = 3
$ gdbg set sum = 100          # change a variable, keep running
$ gdbg step over
```

Watch a value evolve across a loop in **one** call instead of stepping:

```console
$ gdbg trace --pkg . --break main.go:18 --capture sum,it.Qty
trace: 3 hit(s)
 #1   main.total  main.go:18  sum=0  it.Qty=3
 #2   main.total  main.go:18  sum=0  it.Qty=0
 #3   main.total  main.go:18  sum=0  it.Qty=7
```

By default:

- Breakpoints can be line, function, conditional (`--if`), hit-count (`--hit`),
  logpoint (`--log`), or watchpoint (break the instant a value changes).
- Unrecovered panics and fatal errors **always** stop the program, landing on
  the first non-runtime frame with the panic message and the arguments that
  caused it.
- Locals print as real Go values — slices, maps, structs, and interfaces
  render readably, and each stop shows only the locals that *changed*
  (`~ sum = 6 (was 3)`).
- Goroutines are first-class: list them, select one, and inspect its stack.
- One paused process per project is held open between calls, so state survives
  across commands; the daemon shuts down after 30 minutes idle.
- `where` / `def` / `hover` / `refs` navigation works alongside the live
  session (via `go/parser` and `gopls`).

## Why?

An agent editing Go can read the source but not the run. It sees that
`parseConfig` exists; it can't see that `threads` came back `0`. The usual
workaround is to add `fmt.Println`, rebuild, read the log, and delete it — a
slow loop that only shows what you thought to print.

`gdbg` gives the agent the other half: break where a value is computed, read
the real inputs, step to watch it go wrong, and change a variable in place to
test a fix. Continue into a panic to land on the frame that raised it, with
its arguments. Watch a variable to stop the instant it changes.

## Install

```sh
go install github.com/felixbrock/golang-debugger-skill/cmd/gdbg@latest
```

Or from a checkout: `./install.sh` (builds with the local toolchain).

It needs on `PATH`:

- `dlv` — `go install github.com/go-delve/delve/cmd/dlv@latest`
- `gopls` (optional, for `def`/`hover`/`refs`) —
  `go install golang.org/x/tools/gopls@latest`

Recent Delve releases require a matching Go toolchain (dlv 1.25 needs
Go ≥ 1.22). If your system Go is older, `GOTOOLCHAIN=go1.24.4 gdbg …` works.

`--pkg` and `--test` sessions are built by Delve with optimizations and
inlining disabled automatically. For `--bin-path`, build the binary with
`go build -gcflags="all=-N -l"`.

## Use it as a skill

`gdbg` is the whole interface. Drop [`skill/go-debugger`](skill/go-debugger/SKILL.md)
into `.claude/skills/` (or `.agents/skills/` for Codex) and the agent drives
the CLI directly. Run `gdbg` with no arguments for the full command list.

```sh
gdbg where parseConfig                             # find where to break
gdbg launch --pkg . --break main.go:88 -- --threads 4   # build and run to it
gdbg vars ; gdbg eval cfg.Threads sum ; gdbg step over
gdbg set cfg.Threads = 4 --then continue           # test a fix live
gdbg trace --pkg . --break main.go:88 --capture cfg.Threads
gdbg watch cfg.Threads                             # stop when it changes
gdbg launch --test ./pkg -- -run TestFailing       # debug a failing test
```

## Use it as an MCP server

The same binary runs an MCP server (`gdbg mcp`) exposing 28 tools —
`debug_launch`, `debug_step`, `debug_vars`, `debug_eval`, `debug_set`,
`debug_goroutines`, `debug_where`, and the rest. The CLI and MCP share the
same session.

Claude Code — `.mcp.json` in your project, or `claude mcp add godbg -- gdbg mcp`:

```json
{ "mcpServers": { "godbg": { "command": "gdbg", "args": ["mcp"] } } }
```

Codex — `~/.codex/config.toml`:

```toml
[mcp_servers.godbg]
command = "gdbg"
args = ["mcp"]
```

The server picks up the project from the directory it starts in.

## How it works

A per-project daemon holds one paused headless Delve session and serves
commands over a Unix socket. The CLI and the MCP server are both thin clients
of that daemon, so a breakpoint set in one call is still there in the next and
the program stays paused between an agent's tool calls. gdbg speaks Delve's
native JSON-RPC API directly (stdlib only — the module has zero dependencies).
Logs live in `.gdbg/` — add it to `.gitignore`.

The daemon also records every command it executes to `.gdbg/usage.jsonl`
(one JSON line per command: timestamp, command, args, ok/error, duration).
`gdbg usage` prints a per-command frequency summary from that file — useful
for seeing which debugger features agents actually reach for. Purely local;
nothing leaves the machine.

## Limitations

- `set` takes numeric, bool, and pointer values (a Delve limit); `eval`
  takes most Go expressions but cannot call arbitrary functions.
- Watchpoints are scope-bound to the frame where they are set and need
  hardware support (fine on linux/amd64 and arm64).
- There is no reverse debugging; `restart` relaunches the program
  (`--rebuild` picks up source edits).
- `continue` on a program that never stops is auto-interrupted after 25s and
  reports where every goroutine is — use `gdbg goroutines` from there.

## Build

```sh
go build ./cmd/gdbg     # or: go test ./...
```

## License

MIT
