// Package mcp exposes gdbg as an MCP stdio server (newline-delimited
// JSON-RPC 2.0). Every tool is a thin wrapper over the daemon commands, so
// the CLI and MCP share one live session.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/felixbrock/golang-debugger-skill/internal/client"
	"github.com/felixbrock/golang-debugger-skill/internal/nav"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	run         func(root string, args map[string]any) (string, error) `json:"-"`
}

func obj(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func str(desc string) map[string]any  { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any  { return map[string]any{"type": "integer", "description": desc} }
func boolp(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }
func arr(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

// forward builds an argv from tool args and sends it to the daemon.
func forward(build func(a map[string]any) ([]string, error)) func(string, map[string]any) (string, error) {
	return func(root string, a map[string]any) (string, error) {
		argv, err := build(a)
		if err != nil {
			return "", err
		}
		resp, err := client.Send(root, argv)
		if err != nil {
			return "", err
		}
		if resp.IsError {
			return "", fmt.Errorf("%s", resp.Text)
		}
		return resp.Text, nil
	}
}

func s(a map[string]any, key string) string {
	if v, ok := a[key].(string); ok {
		return v
	}
	if v, ok := a[key].(float64); ok {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func b(a map[string]any, key string) bool { v, _ := a[key].(bool); return v }

func list(a map[string]any, key string) []string {
	raw, _ := a[key].([]any)
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if v, ok := r.(string); ok {
			out = append(out, v)
		}
	}
	return out
}

func launchArgv(cmd string, a map[string]any) ([]string, error) {
	argv := []string{cmd}
	if v := s(a, "pkg"); v != "" {
		argv = append(argv, "--pkg", v)
	}
	if v := s(a, "test"); v != "" {
		argv = append(argv, "--test", v)
	}
	if v := s(a, "bin_path"); v != "" {
		argv = append(argv, "--bin-path", v)
	}
	for _, l := range list(a, "breakpoints") {
		argv = append(argv, "--break", l)
	}
	for _, f := range list(a, "break_functions") {
		argv = append(argv, "--break-fn", f)
	}
	if v := s(a, "build_flags"); v != "" {
		argv = append(argv, "--build-flags", v)
	}
	if cmd == "trace" {
		if v := s(a, "capture"); v != "" {
			argv = append(argv, "--capture", v)
		}
		if v := s(a, "max_hits"); v != "" {
			argv = append(argv, "--max", v)
		}
	}
	if cmd == "launch" && b(a, "stop_entry") {
		argv = append(argv, "--stop-entry")
	}
	if args := list(a, "args"); len(args) > 0 {
		argv = append(argv, "--")
		argv = append(argv, args...)
	}
	return argv, nil
}

var launchProps = map[string]any{
	"pkg":             str("package dir to build & debug (dlv debug), e.g. \".\" or \"./cmd/app\""),
	"test":            str("package dir to debug its tests (dlv test)"),
	"bin_path":        str("pre-built binary to debug (dlv exec); build with -gcflags=\"all=-N -l\""),
	"breakpoints":     arr("breakpoint locations, e.g. [\"main.go:12\", \"pkg/x.go:40\"]"),
	"break_functions": arr("function names to break on, e.g. [\"main.total\"]"),
	"args":            arr("program arguments (for --test: -run/-v are translated to -test.run/-test.v)"),
	"build_flags":     str("extra go build flags"),
}

func tools() []tool {
	mergedLaunch := map[string]any{}
	for k, v := range launchProps {
		mergedLaunch[k] = v
	}
	mergedLaunch["stop_entry"] = boolp("stay paused before main even when breakpoints are set")

	traceProps := map[string]any{}
	for k, v := range launchProps {
		traceProps[k] = v
	}
	traceProps["capture"] = str("comma-separated expressions to record at each hit, e.g. \"i,sum\"")
	traceProps["max_hits"] = num("stop after this many hits (default 30)")

	return []tool{
		{"debug_launch", "Build and run a Go program or test under the debugger, paused. Exactly one of pkg/test/bin_path. With breakpoints it runs to the first stop; unrecovered panics always stop. One session per project; a new launch replaces it.", obj(mergedLaunch), forward(func(a map[string]any) ([]string, error) { return launchArgv("launch", a) })},
		{"debug_trace", "Run through every hit of a breakpoint capturing expressions, in one call — see a value evolve across a loop without stepping.", obj(traceProps, "breakpoints"), forward(func(a map[string]any) ([]string, error) { return launchArgv("trace", a) })},
		{"debug_break", "Set a breakpoint: location \"file:line\" or a function name; optional condition, hit count, or log template (logpoint prints and continues).", obj(map[string]any{
			"location":  str("file:line, e.g. \"main.go:42\""),
			"function":  str("function name, e.g. \"main.total\" (alternative to location)"),
			"condition": str("Go boolean expression, e.g. \"i == 5\""),
			"hit_count": str("stop on Nth hit, e.g. \"3\" or \"% 2\""),
			"log":       str("logpoint template, e.g. \"i={i} sum={sum}\" — prints and continues"),
		}), forward(func(a map[string]any) ([]string, error) {
			argv := []string{"break"}
			if v := s(a, "location"); v != "" {
				argv = append(argv, v)
			}
			if v := s(a, "function"); v != "" {
				argv = append(argv, "--fn", v)
			}
			if v := s(a, "condition"); v != "" {
				argv = append(argv, "--if", v)
			}
			if v := s(a, "hit_count"); v != "" {
				argv = append(argv, "--hit", v)
			}
			if v := s(a, "log"); v != "" {
				argv = append(argv, "--log", v)
			}
			return argv, nil
		})},
		{"debug_breaks", "List breakpoints and watchpoints with ids and hit counts.", obj(nil), forward(func(a map[string]any) ([]string, error) { return []string{"breaks"}, nil })},
		{"debug_break_remove", "Remove a breakpoint by id.", obj(map[string]any{"id": num("breakpoint id")}, "id"), forward(func(a map[string]any) ([]string, error) { return []string{"break-rm", s(a, "id")}, nil })},
		{"debug_break_toggle", "Enable or disable a breakpoint by id.", obj(map[string]any{"id": num("breakpoint id"), "enable": boolp("true to enable, false to disable")}, "id"), forward(func(a map[string]any) ([]string, error) {
			cmd := "break-off"
			if b(a, "enable") {
				cmd = "break-on"
			}
			return []string{cmd, s(a, "id")}, nil
		})},
		{"debug_watch", "Set a watchpoint: stop the moment a variable is written (or read). Scope-bound to the current frame.", obj(map[string]any{"variable": str("variable to watch"), "read": boolp("also stop on reads")}, "variable"), forward(func(a map[string]any) ([]string, error) {
			argv := []string{"watch", s(a, "variable")}
			if b(a, "read") {
				argv = append(argv, "--read")
			}
			return argv, nil
		})},
		{"debug_continue", "Resume until the next breakpoint, watchpoint, panic, or exit. Interrupts and reports after 25s if still running.", obj(nil), forward(func(a map[string]any) ([]string, error) { return []string{"continue"}, nil })},
		{"debug_step", "Step: over (next line), in (into calls), out (finish function, shows return values), insn (one instruction).", obj(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"over", "in", "out", "insn"}, "description": "step kind"}}, "kind"), forward(func(a map[string]any) ([]string, error) { return []string{"step", s(a, "kind")}, nil })},
		{"debug_until", "Run to a specific line (temporary breakpoint).", obj(map[string]any{"location": str("file:line")}, "location"), forward(func(a map[string]any) ([]string, error) { return []string{"until", s(a, "location")}, nil })},
		{"debug_pause", "Interrupt a running program and show where every goroutine is.", obj(nil), forward(func(a map[string]any) ([]string, error) { return []string{"pause"}, nil })},
		{"debug_restart", "Relaunch the target from the start, keeping breakpoints. rebuild=true recompiles first.", obj(map[string]any{"rebuild": boolp("recompile before restarting")}), forward(func(a map[string]any) ([]string, error) {
			argv := []string{"restart"}
			if b(a, "rebuild") {
				argv = append(argv, "--rebuild")
			}
			return argv, nil
		})},
		{"debug_vars", "Show function arguments and locals of the selected frame as real Go values.", obj(map[string]any{"depth": num("max recursion into nested values (default 2)")}), forward(func(a map[string]any) ([]string, error) {
			argv := []string{"vars"}
			if v := s(a, "depth"); v != "" {
				argv = append(argv, "--depth", v)
			}
			return argv, nil
		})},
		{"debug_eval", "Evaluate Go expressions in the selected frame: variable paths (cfg.Threads, items[0].Qty), comparisons, len/cap, etc.", obj(map[string]any{"expressions": arr("expressions to evaluate")}, "expressions"), forward(func(a map[string]any) ([]string, error) {
			return append([]string{"eval"}, list(a, "expressions")...), nil
		})},
		{"debug_set", "Change a variable in the running process (numeric, bool, pointer) to test a fix without recompiling.", obj(map[string]any{
			"variable":      str("variable path, e.g. cfg.Threads"),
			"value":         str("new value"),
			"then_continue": boolp("resume right after setting"),
		}, "variable", "value"), forward(func(a map[string]any) ([]string, error) {
			argv := []string{"set", s(a, "variable"), "=", s(a, "value")}
			if b(a, "then_continue") {
				argv = append(argv, "--then", "continue")
			}
			return argv, nil
		})},
		{"debug_watch_expr", "Manage expressions re-evaluated and shown at every stop.", obj(map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"add", "rm", "list"}},
			"expr":   str("expression (for add/rm)"),
		}, "action"), forward(func(a map[string]any) ([]string, error) {
			argv := []string{"watch-expr", s(a, "action")}
			if v := s(a, "expr"); v != "" {
				argv = append(argv, v)
			}
			return argv, nil
		})},
		{"debug_backtrace", "Backtrace of the selected goroutine.", obj(map[string]any{"depth": num("max frames (default 20)")}), forward(func(a map[string]any) ([]string, error) {
			argv := []string{"bt"}
			if v := s(a, "depth"); v != "" {
				argv = append(argv, "--depth", v)
			}
			return argv, nil
		})},
		{"debug_list", "Show source around the current line of the selected frame.", obj(nil), forward(func(a map[string]any) ([]string, error) { return []string{"list"}, nil })},
		{"debug_state", "Current stop location plus all locals and watch expressions in one call.", obj(nil), forward(func(a map[string]any) ([]string, error) { return []string{"state"}, nil })},
		{"debug_goroutines", "List goroutines with their current user location.", obj(nil), forward(func(a map[string]any) ([]string, error) { return []string{"goroutines"}, nil })},
		{"debug_goroutine", "Select a goroutine; vars/eval/backtrace follow it.", obj(map[string]any{"id": num("goroutine id")}, "id"), forward(func(a map[string]any) ([]string, error) { return []string{"goroutine", s(a, "id")}, nil })},
		{"debug_frame", "Select a stack frame by number, or move up/down; vars/eval follow it.", obj(map[string]any{"selector": str("frame number, \"up\", or \"down\"")}, "selector"), forward(func(a map[string]any) ([]string, error) { return []string{"frame", s(a, "selector")}, nil })},
		{"debug_output", "Show the target program's captured stdout/stderr.", obj(nil), forward(func(a map[string]any) ([]string, error) { return []string{"output"}, nil })},
		{"debug_stop", "End the debug session (kills the target; daemon stays for the next launch).", obj(nil), forward(func(a map[string]any) ([]string, error) { return []string{"stop"}, nil })},
		{"debug_where", "Find where a symbol (function, method, type, const, var) is declared in the project — no session needed.", obj(map[string]any{"name": str("symbol name, e.g. \"parseConfig\"")}, "name"), func(root string, a map[string]any) (string, error) {
			return nav.Where(root, s(a, "name"))
		}},
		{"debug_definition", "Jump to definition of the symbol at file:line:col (gopls).", navSchema(), navRun("definition")},
		{"debug_hover", "Type/doc info for the symbol at file:line:col (gopls).", navSchema(), navRun("hover")},
		{"debug_references", "All references to the symbol at file:line:col (gopls).", navSchema(), navRun("references")},
	}
}

func navSchema() map[string]any {
	return obj(map[string]any{
		"file": str("path relative to the project root"),
		"line": num("1-based line"),
		"col":  num("1-based column"),
	}, "file", "line", "col")
}

func navRun(verb string) func(string, map[string]any) (string, error) {
	return func(root string, a map[string]any) (string, error) {
		line, _ := strconv.Atoi(s(a, "line"))
		col, _ := strconv.Atoi(s(a, "col"))
		return nav.Gopls(root, verb, s(a, "file"), line, col)
	}
}

// Serve runs the MCP server on stdio until EOF.
func Serve(root string) error {
	all := tools()
	byName := map[string]tool{}
	for _, t := range all {
		byName[t.Name] = t
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := json.NewEncoder(os.Stdout)

	reply := func(id json.RawMessage, result any, err *rpcError) {
		if id == nil {
			return
		}
		out.Encode(response{JSONRPC: "2.0", ID: id, Result: result, Error: err})
	}

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		switch req.Method {
		case "initialize":
			reply(req.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "gdbg", "version": version()},
			}, nil)
		case "ping":
			reply(req.ID, map[string]any{}, nil)
		case "tools/list":
			reply(req.ID, map[string]any{"tools": all}, nil)
		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			json.Unmarshal(req.Params, &params)
			t, ok := byName[params.Name]
			if !ok {
				reply(req.ID, nil, &rpcError{Code: -32602, Message: "unknown tool " + params.Name})
				continue
			}
			text, err := t.run(root, params.Arguments)
			isErr := false
			if err != nil {
				text, isErr = err.Error(), true
			}
			reply(req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": text}},
				"isError": isErr,
			}, nil)
		default:
			// Notifications (no id) are ignored; unknown requests get an error.
			reply(req.ID, nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method})
		}
	}
	return in.Err()
}

func version() string { return "0.1.0" }
