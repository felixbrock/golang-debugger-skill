// gdbg — debug a Go program from a coding agent.
//
// A thin client: most commands are forwarded to a per-project daemon that
// holds one paused Delve session, so breakpoints and process state survive
// across calls. Navigation commands run in-process.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/felixbrock/golang-debugger-skill/internal/client"
	"github.com/felixbrock/golang-debugger-skill/internal/daemon"
	"github.com/felixbrock/golang-debugger-skill/internal/mcp"
	"github.com/felixbrock/golang-debugger-skill/internal/nav"
)

const usage = `gdbg — debug a Go program: breakpoints, stepping, real values, live edits.

Session (state persists between calls; one paused process per project):
  gdbg launch --pkg <dir> [--break file:line]... [--break-fn name]... [-- args]
  gdbg launch --test <dir> [--break file:line]... [-- -run TestName]
  gdbg launch --bin-path <file> [--break file:line]...     # pre-built binary
  gdbg trace  --pkg <dir> --break file:line --capture i,sum [--max 30]
  gdbg restart [--rebuild]      relaunch (rebuild picks up code edits)
  gdbg stop                     end the session;  gdbg down  stops the daemon

Breakpoints (set any time; panics and fatal errors always stop):
  gdbg break file.go:42 [--if "i == 5"] [--hit 3] [--log "i={i}"]
  gdbg break --fn main.total
  gdbg watch <var> [--read]     stop the moment a variable changes
  gdbg breaks | break-rm <id> | break-on <id> | break-off <id>

Run:
  gdbg continue                 to next stop (auto-interrupts after 25s)
  gdbg step over|in|out|insn
  gdbg until file.go:99
  gdbg pause

Inspect and change state:
  gdbg vars [--depth N]         args + locals as real Go values
  gdbg eval <expr>...           e.g.  gdbg eval items[0].Qty len(items)==2
  gdbg set <var> = <value> [--then continue]
  gdbg watch-expr add|rm <expr> | watch-expr list
  gdbg bt [--depth N] | list | state | output

Goroutines and frames (vars/eval follow the selection):
  gdbg goroutines | goroutine <id> | frame <n>|up|down

Navigate (no session needed):
  gdbg where <name>             find a declaration
  gdbg def|hover|refs <file> <line> <col>       via gopls

Several commands in one call:
  gdbg do "vars; step over; vars"

MCP server (same session as the CLI):
  gdbg mcp
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(usage)
		return
	}

	root := client.ProjectRoot(".")

	switch args[0] {
	case "__daemon":
		if len(args) > 1 {
			root = args[1]
		}
		if err := daemon.Run(root); err != nil {
			fmt.Fprintln(os.Stderr, "daemon:", err)
			os.Exit(1)
		}
		return

	case "mcp":
		if err := mcp.Serve(root); err != nil {
			fmt.Fprintln(os.Stderr, "mcp:", err)
			os.Exit(1)
		}
		return

	case "where":
		if len(args) != 2 {
			die("usage: gdbg where <name>")
		}
		text, err := nav.Where(root, args[1])
		finish(text, err)
		return

	case "def", "hover", "refs":
		if len(args) != 4 {
			die("usage: gdbg " + args[0] + " <file> <line> <col>")
		}
		verb := map[string]string{"def": "definition", "hover": "hover", "refs": "references"}[args[0]]
		line, err1 := strconv.Atoi(args[2])
		col, err2 := strconv.Atoi(args[3])
		if err1 != nil || err2 != nil {
			die("line and col must be numbers")
		}
		text, err := nav.Gopls(root, verb, args[1], line, col)
		finish(text, err)
		return

	case "down", "stop":
		resp, err := client.SendIfRunning(root, args)
		if err != nil {
			fmt.Println("no daemon running")
			return
		}
		fmt.Println(resp.Text)
		return
	}

	resp, err := client.Send(root, args)
	if err != nil {
		die(err.Error())
	}
	if resp.IsError {
		fmt.Fprintln(os.Stderr, resp.Text)
		os.Exit(1)
	}
	fmt.Println(resp.Text)
}

func finish(text string, err error) {
	if err != nil {
		die(err.Error())
	}
	fmt.Println(text)
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
}
