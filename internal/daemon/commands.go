package daemon

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/felixbrock/golang-debugger-skill/internal/dlv"
)

// dispatch executes one gdbg command against the daemon.
func (d *Daemon) dispatch(argv []string) (string, error) {
	if len(argv) == 0 {
		return "", fmt.Errorf("empty command")
	}
	cmd, args := argv[0], argv[1:]

	switch cmd {
	case "launch":
		return d.cmdLaunch(args)
	case "trace":
		return d.cmdTrace(args)
	case "do":
		return d.cmdDo(args)
	case "down":
		return "", nil // handled by the serve loop
	}

	s := d.session()
	if s == nil {
		return "", fmt.Errorf("no debug session; start one with: gdbg launch --pkg <dir> --break <file:line>")
	}

	switch cmd {
	case "break":
		return s.cmdBreak(args)
	case "breaks":
		return s.cmdBreaks()
	case "break-rm":
		return s.cmdBreakRm(args)
	case "break-on", "break-off":
		return s.cmdBreakToggle(cmd == "break-off", args)
	case "watch":
		return s.cmdWatch(args)
	case "continue", "c":
		return s.resume(dlv.Continue)
	case "step":
		return s.cmdStep(args)
	case "until":
		return s.cmdUntil(args)
	case "pause":
		return s.cmdPause()
	case "restart":
		return s.cmdRestart(args)
	case "vars":
		return s.cmdVars(args)
	case "eval":
		return s.cmdEval(args)
	case "set":
		return s.cmdSet(args)
	case "watch-expr":
		return s.cmdWatchExpr(args)
	case "bt":
		return s.cmdBt(args)
	case "list":
		return s.cmdList()
	case "state":
		return s.cmdState()
	case "goroutines":
		return s.cmdGoroutines()
	case "goroutine":
		return s.cmdGoroutine(args)
	case "frame":
		return s.cmdFrame(args)
	case "output":
		return strings.Join(s.proc.AllOutput(), "\n"), nil
	case "stop":
		d.stopSession()
		return "session ended (daemon still up; gdbg down stops it)", nil
	default:
		return "", fmt.Errorf("unknown command %q; run gdbg with no arguments for help", cmd)
	}
}

// ---------- flag parsing ----------

// parseArgs splits argv into flags (per spec: true = takes a value),
// positionals, and everything after "--".
func parseArgs(args []string, spec map[string]bool) (map[string][]string, []string, []string, error) {
	flags := map[string][]string{}
	var pos, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = args[i+1:]
			break
		}
		if strings.HasPrefix(a, "--") {
			name := strings.TrimPrefix(a, "--")
			val := ""
			if eq := strings.Index(name, "="); eq >= 0 {
				name, val = name[:eq], name[eq+1:]
				if _, ok := spec[name]; !ok {
					return nil, nil, nil, fmt.Errorf("unknown flag --%s", name)
				}
				flags[name] = append(flags[name], val)
				continue
			}
			takesValue, ok := spec[name]
			if !ok {
				return nil, nil, nil, fmt.Errorf("unknown flag --%s", name)
			}
			if takesValue {
				if i+1 >= len(args) {
					return nil, nil, nil, fmt.Errorf("--%s needs a value", name)
				}
				i++
				val = args[i]
			}
			flags[name] = append(flags[name], val)
			continue
		}
		pos = append(pos, a)
	}
	return flags, pos, rest, nil
}

func first(flags map[string][]string, name string) string {
	if v := flags[name]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func has(flags map[string][]string, name string) bool {
	_, ok := flags[name]
	return ok
}

// ---------- launch / trace ----------

var launchSpec = map[string]bool{
	"pkg": true, "test": true, "bin-path": true, "build-flags": true,
	"break": true, "break-fn": true, "stop-entry": false,
	// trace-only:
	"capture": true, "max": true,
}

func launchMode(flags map[string][]string) (mode, target string, err error) {
	n := 0
	if v := first(flags, "pkg"); v != "" {
		mode, target, n = "debug", v, n+1
	}
	if v := first(flags, "test"); v != "" {
		mode, target, n = "test", v, n+1
	}
	if v := first(flags, "bin-path"); v != "" {
		mode, target, n = "exec", v, n+1
	}
	if n != 1 {
		return "", "", fmt.Errorf("need exactly one of --pkg <dir>, --test <dir>, --bin-path <file>")
	}
	return mode, target, nil
}

// normalizeTestArgs lets users write familiar `-run X -v` for test binaries.
func normalizeTestArgs(mode string, args []string) []string {
	if mode != "test" {
		return args
	}
	out := make([]string, 0, len(args))
	for _, a := range args {
		for _, f := range []string{"run", "v", "count", "bench", "timeout"} {
			if a == "-"+f || strings.HasPrefix(a, "-"+f+"=") {
				a = "-test." + strings.TrimPrefix(a, "-")
				break
			}
		}
		out = append(out, a)
	}
	return out
}

func (d *Daemon) cmdLaunch(args []string) (string, error) {
	flags, pos, rest, err := parseArgs(args, launchSpec)
	if err != nil {
		return "", err
	}
	if len(pos) > 0 {
		return "", fmt.Errorf("unexpected argument %q (program args go after --)", pos[0])
	}
	mode, target, err := launchMode(flags)
	if err != nil {
		return "", err
	}
	d.stopSession()
	s, err := newSession(d.root, mode, target, first(flags, "build-flags"),
		normalizeTestArgs(mode, rest), d.reqEnv)
	if err != nil {
		return "", err
	}
	d.setSession(s)

	var msgs []string
	nbreaks := 0
	for _, loc := range flags["break"] {
		bp, err := s.createBreakpoint(loc, "", "", "")
		if err != nil {
			d.stopSession()
			return "", fmt.Errorf("breakpoint %s: %w", loc, err)
		}
		msgs = append(msgs, describeBreak(bp))
		nbreaks++
	}
	for _, fn := range flags["break-fn"] {
		bp, err := s.createBreakpoint(fn, "", "", "")
		if err != nil {
			d.stopSession()
			return "", fmt.Errorf("breakpoint on %s: %w", fn, err)
		}
		msgs = append(msgs, describeBreak(bp))
		nbreaks++
	}

	head := strings.Join(msgs, "\n")
	if nbreaks == 0 || has(flags, "stop-entry") {
		if head != "" {
			head += "\n"
		}
		return head + "launched, paused before main. Set breakpoints, then gdbg continue.\n" +
			"(unrecovered panics and fatal errors always stop the program)", nil
	}
	report, err := s.resume(dlv.Continue)
	if err != nil {
		return "", err
	}
	if head != "" {
		return head + "\n" + report, nil
	}
	return report, nil
}

func (d *Daemon) cmdTrace(args []string) (string, error) {
	flags, _, rest, err := parseArgs(args, launchSpec)
	if err != nil {
		return "", err
	}
	locs := flags["break"]
	if len(locs) == 0 {
		return "", fmt.Errorf("trace needs --break <file:line>")
	}
	captures := splitList(first(flags, "capture"))
	max := 30
	if m := first(flags, "max"); m != "" {
		if max, err = strconv.Atoi(m); err != nil {
			return "", fmt.Errorf("--max: %w", err)
		}
	}
	mode, target, err := launchMode(flags)
	if err != nil {
		return "", err
	}
	d.stopSession()
	s, err := newSession(d.root, mode, target, first(flags, "build-flags"),
		normalizeTestArgs(mode, rest), d.reqEnv)
	if err != nil {
		return "", err
	}
	d.setSession(s)
	for _, loc := range locs {
		if _, err := s.createBreakpoint(loc, "", "", ""); err != nil {
			d.stopSession()
			return "", fmt.Errorf("breakpoint %s: %w", loc, err)
		}
	}

	var b strings.Builder
	hits := 0
	for hits < max {
		state, interrupted, err := s.runCommand(dlv.Continue)
		if err != nil {
			if strings.Contains(err.Error(), "has exited") {
				break
			}
			return "", err
		}
		if state.Exited {
			break
		}
		th := state.CurrentThread
		if interrupted || th == nil {
			b.WriteString("(target did not reach the breakpoint before timeout; halted)\n")
			break
		}
		if bp := th.Breakpoint; bp != nil && (bp.Name == "unrecovered-panic" || bp.Name == "fatal-throw") {
			hits++
			fmt.Fprintf(&b, " #%-3d PANIC at %s:%d — gdbg state to inspect\n", hits, filepath.Base(th.File), th.Line)
			s.selG, s.selFrame = -1, 0
			return fmt.Sprintf("trace: %d hit(s)\n%s", hits, strings.TrimRight(b.String(), "\n")), nil
		}
		hits++
		fn := "?"
		if th.Function != nil {
			fn = th.Function.Name
		}
		row := fmt.Sprintf(" #%-3d %s  %s:%d", hits, fn, filepath.Base(th.File), th.Line)
		for _, cap := range captures {
			v, err := s.c.Eval(dlv.EvalScope{GoroutineID: -1}, cap, &dlv.DefaultLoad)
			if err != nil {
				row += fmt.Sprintf("  %s=<err>", cap)
				continue
			}
			row += fmt.Sprintf("  %s=%s", cap, inlineVar(*v, 60))
		}
		b.WriteString(row + "\n")
	}
	s.exited = true // trace runs the program to completion (or max hits)
	suffix := ""
	if hits >= max {
		suffix = fmt.Sprintf(" (stopped at --max %d; program halted mid-run)", max)
		s.exited = false
	}
	writeOutput(&b, s.proc.NewOutput())
	return fmt.Sprintf("trace: %d hit(s)%s\n%s", hits, suffix, strings.TrimRight(b.String(), "\n")), nil
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// cmdDo runs several commands in one call: gdbg do "vars; step over; vars"
func (d *Daemon) cmdDo(args []string) (string, error) {
	script := strings.Join(args, " ")
	var out []string
	for _, part := range strings.Split(script, ";") {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		res, err := d.dispatch(fields)
		if err != nil {
			out = append(out, fmt.Sprintf("$ %s\nerror: %v", strings.Join(fields, " "), err))
			break
		}
		out = append(out, fmt.Sprintf("$ %s\n%s", strings.Join(fields, " "), res))
	}
	return strings.Join(out, "\n\n"), nil
}

// ---------- breakpoints ----------

func (s *Session) createBreakpoint(loc, cond, hit, log string) (*dlv.Breakpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bp, err := s.c.CreateBreakpoint(&dlv.Breakpoint{Cond: cond, HitCond: hit}, loc)
	if err != nil {
		return nil, err
	}
	if log != "" {
		s.logpoints[bp.ID] = log
	}
	return bp, nil
}

func describeBreak(bp *dlv.Breakpoint) string {
	fn := bp.FunctionName
	if fn != "" {
		fn = "  in " + fn
	}
	return fmt.Sprintf("breakpoint %d set at %s:%d%s", bp.ID, filepath.Base(bp.File), bp.Line, fn)
}

var breakSpec = map[string]bool{"if": true, "hit": true, "log": true, "fn": true}

func (s *Session) cmdBreak(args []string) (string, error) {
	flags, pos, _, err := parseArgs(args, breakSpec)
	if err != nil {
		return "", err
	}
	loc := first(flags, "fn")
	if loc == "" {
		if len(pos) != 1 {
			return "", fmt.Errorf("usage: gdbg break <file:line> [--if cond] [--hit N] [--log \"msg {expr}\"] | gdbg break --fn <func>")
		}
		loc = pos[0]
	}
	hit := first(flags, "hit")
	if hit != "" && !strings.ContainsAny(hit, "<>=%") {
		hit = "== " + hit
	}
	bp, err := s.createBreakpoint(loc, first(flags, "if"), hit, first(flags, "log"))
	if err != nil {
		return "", err
	}
	desc := describeBreak(bp)
	if _, ok := s.logpoints[bp.ID]; ok {
		desc += "  (logpoint: prints and continues)"
	}
	return desc, nil
}

func (s *Session) cmdBreaks() (string, error) {
	bps, err := s.c.ListBreakpoints()
	if err != nil {
		return "", err
	}
	sort.Slice(bps, func(i, j int) bool { return bps[i].ID < bps[j].ID })
	var b strings.Builder
	for _, bp := range bps {
		if bp.ID < 0 { // internal: unrecovered-panic, fatal-throw
			continue
		}
		state := ""
		if bp.Disabled {
			state = "  [disabled]"
		}
		extra := ""
		if bp.Cond != "" {
			extra += fmt.Sprintf("  if %s", bp.Cond)
		}
		if bp.HitCond != "" {
			extra += fmt.Sprintf("  hit %s", bp.HitCond)
		}
		if tmpl, ok := s.logpoints[bp.ID]; ok {
			extra += fmt.Sprintf("  log %q", tmpl)
		}
		if bp.WatchExpr != "" {
			fmt.Fprintf(&b, "  %d: watchpoint %s  (hits: %d)%s\n", bp.ID, bp.WatchExpr, bp.TotalHitCount, state)
			continue
		}
		fmt.Fprintf(&b, "  %d: %s:%d%s  (hits: %d)%s\n",
			bp.ID, filepath.Base(bp.File), bp.Line, extra, bp.TotalHitCount, state)
	}
	if b.Len() == 0 {
		return "no breakpoints (panic and fatal-error stops are always on)", nil
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (s *Session) cmdBreakRm(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("usage: gdbg break-rm <id>")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return "", err
	}
	if _, err := s.c.ClearBreakpoint(id); err != nil {
		return "", err
	}
	delete(s.logpoints, id)
	return fmt.Sprintf("breakpoint %d removed", id), nil
}

func (s *Session) cmdBreakToggle(disable bool, args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("usage: gdbg break-on|break-off <id>")
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return "", err
	}
	bps, err := s.c.ListBreakpoints()
	if err != nil {
		return "", err
	}
	for _, bp := range bps {
		if bp.ID == id {
			bp.Disabled = disable
			if err := s.c.AmendBreakpoint(bp); err != nil {
				return "", err
			}
			verb := "enabled"
			if disable {
				verb = "disabled"
			}
			return fmt.Sprintf("breakpoint %d %s", id, verb), nil
		}
	}
	return "", fmt.Errorf("no breakpoint %d", id)
}

func (s *Session) cmdWatch(args []string) (string, error) {
	flags, pos, _, err := parseArgs(args, map[string]bool{"read": false})
	if err != nil {
		return "", err
	}
	if len(pos) != 1 {
		return "", fmt.Errorf("usage: gdbg watch <variable> [--read]")
	}
	typ := dlv.WatchWrite
	if has(flags, "read") {
		typ |= dlv.WatchRead
	}
	bp, err := s.c.CreateWatchpoint(s.scope(), pos[0], typ)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("watchpoint %d set on %s (stops when it changes; scope-bound)", bp.ID, pos[0]), nil
}

// ---------- execution ----------

func (s *Session) cmdStep(args []string) (string, error) {
	kind := "over"
	if len(args) > 0 {
		kind = args[0]
	}
	name := map[string]string{
		"over": dlv.Next, "in": dlv.Step, "into": dlv.Step,
		"out": dlv.StepOut, "insn": dlv.StepInstruction,
	}[kind]
	if name == "" {
		return "", fmt.Errorf("usage: gdbg step over|in|out|insn")
	}
	return s.resume(name)
}

func (s *Session) cmdUntil(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("usage: gdbg until <file:line>")
	}
	bp, err := s.createBreakpoint(args[0], "", "", "")
	if err != nil {
		return "", err
	}
	report, err := s.resume(dlv.Continue)
	s.c.ClearBreakpoint(bp.ID) // best effort; harmless if the program exited
	return report, err
}

func (s *Session) cmdPause() (string, error) {
	state, err := s.c.Halt()
	if err != nil {
		return "", err
	}
	s.selG, s.selFrame = -1, 0
	return s.renderStop(state, nil, false), nil
}

func (s *Session) cmdRestart(args []string) (string, error) {
	flags, _, _, err := parseArgs(args, map[string]bool{"rebuild": false})
	if err != nil {
		return "", err
	}
	rebuild := has(flags, "rebuild")
	if s.mode == "exec" && rebuild {
		return "", fmt.Errorf("--rebuild only works for --pkg/--test sessions")
	}
	if err := s.c.Restart(rebuild); err != nil {
		return "", err
	}
	s.exited = false
	s.selG, s.selFrame = -1, 0
	s.lastLocals = map[string]string{}
	bps, _ := s.cmdBreaks()
	return "restarted, paused before main. Breakpoints kept:\n" + bps + "\nRun gdbg continue.", nil
}

// ---------- inspection ----------

func (s *Session) cmdVars(args []string) (string, error) {
	flags, _, _, err := parseArgs(args, map[string]bool{"depth": true})
	if err != nil {
		return "", err
	}
	cfg := dlv.DefaultLoad
	if d := first(flags, "depth"); d != "" {
		if cfg.MaxVariableRecurse, err = strconv.Atoi(d); err != nil {
			return "", err
		}
	}
	vars := s.currentVars(cfg)
	if len(vars) == 0 {
		return "no variables in scope", nil
	}
	return renderVars(vars), nil
}

func (s *Session) cmdEval(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: gdbg eval <expr> [<expr>…]")
	}
	// Support both `eval a b` (two paths) and `eval len(x) == 2` (one expr).
	exprs := args
	if len(args) > 1 && strings.ContainsAny(strings.Join(args, " "), "(=<>+-*/ ") {
		joined := strings.Join(args, " ")
		if !allIdentifiers(args) {
			exprs = []string{joined}
		}
	}
	var b strings.Builder
	for _, expr := range exprs {
		v, err := s.c.Eval(s.scope(), expr, &dlv.DefaultLoad)
		if err != nil {
			fmt.Fprintf(&b, "%s: error: %v\n", expr, err)
			continue
		}
		lbl := expr
		if v.Name != "" {
			lbl = v.Name
		}
		val := renderVars([]dlv.Variable{withName(*v, lbl)})
		b.WriteString(val + "\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func allIdentifiers(args []string) bool {
	for _, a := range args {
		if strings.ContainsAny(a, "(=<>+*/ ") {
			return false
		}
	}
	return true
}

func withName(v dlv.Variable, name string) dlv.Variable {
	v.Name = name
	return v
}

func (s *Session) cmdSet(args []string) (string, error) {
	flags, pos, _, err := parseArgs(args, map[string]bool{"then": true})
	if err != nil {
		return "", err
	}
	stmt := strings.Join(pos, " ")
	eq := strings.Index(stmt, "=")
	if eq < 0 {
		return "", fmt.Errorf("usage: gdbg set <var> = <value> [--then continue]")
	}
	sym := strings.TrimSpace(stmt[:eq])
	val := strings.TrimSpace(stmt[eq+1:])
	if err := s.c.Set(s.scope(), sym, val); err != nil {
		return "", fmt.Errorf("set %s: %w (delve can set numeric, bool and pointer values)", sym, err)
	}
	v, err := s.c.Eval(s.scope(), sym, &dlv.DefaultLoad)
	msg := fmt.Sprintf("%s = %s", sym, val)
	if err == nil {
		msg = fmt.Sprintf("%s = %s", sym, inlineVar(*v, 100))
	}
	if first(flags, "then") == "continue" {
		report, err := s.resume(dlv.Continue)
		if err != nil {
			return "", err
		}
		return msg + "\n" + report, nil
	}
	return msg, nil
}

func (s *Session) cmdWatchExpr(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: gdbg watch-expr add|rm <expr> | watch-expr list")
	}
	switch args[0] {
	case "add":
		expr := strings.Join(args[1:], " ")
		if expr == "" {
			return "", fmt.Errorf("usage: gdbg watch-expr add <expr>")
		}
		s.watchExprs = append(s.watchExprs, expr)
		return fmt.Sprintf("watching %s (re-shown at every stop)", expr), nil
	case "rm":
		expr := strings.Join(args[1:], " ")
		for i, e := range s.watchExprs {
			if e == expr {
				s.watchExprs = append(s.watchExprs[:i], s.watchExprs[i+1:]...)
				return fmt.Sprintf("removed %s", expr), nil
			}
		}
		return "", fmt.Errorf("not watching %q", expr)
	case "list":
		if len(s.watchExprs) == 0 {
			return "no watch expressions", nil
		}
		return "  " + strings.Join(s.watchExprs, "\n  "), nil
	}
	return "", fmt.Errorf("usage: gdbg watch-expr add|rm <expr> | watch-expr list")
}

func (s *Session) cmdBt(args []string) (string, error) {
	flags, _, _, err := parseArgs(args, map[string]bool{"depth": true})
	if err != nil {
		return "", err
	}
	depth := 20
	if d := first(flags, "depth"); d != "" {
		if depth, err = strconv.Atoi(d); err != nil {
			return "", err
		}
	}
	frames, err := s.c.Stacktrace(s.selG, depth, nil)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i, fr := range frames {
		fn := "?"
		if fr.Function != nil {
			fn = fr.Function.Name
		}
		marker := "  "
		if i == s.selFrame {
			marker = "* "
		}
		fmt.Fprintf(&b, "%s%2d  %s  %s:%d\n", marker, i, fn, filepath.Base(fr.File), fr.Line)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (s *Session) cmdList() (string, error) {
	file, line, err := s.currentLocation()
	if err != nil {
		return "", err
	}
	return sourceContext(file, line, 6), nil
}

func (s *Session) currentLocation() (string, int, error) {
	frames, err := s.c.Stacktrace(s.selG, s.selFrame+1, nil)
	if err != nil {
		return "", 0, err
	}
	if s.selFrame >= len(frames) {
		return "", 0, fmt.Errorf("frame %d out of range", s.selFrame)
	}
	fr := frames[s.selFrame]
	return fr.File, fr.Line, nil
}

func (s *Session) cmdState() (string, error) {
	state, err := s.c.State(false)
	if err != nil {
		return "", err
	}
	if state == nil {
		return "", fmt.Errorf("no state")
	}
	var b strings.Builder
	// Reuse the stop rendering, but do not consume the locals delta.
	saved := s.lastLocals
	s.lastLocals = map[string]string{}
	b.WriteString(s.renderStop(state, nil, false))
	s.lastLocals = saved
	b.WriteString("\n")
	vars := s.currentVars(dlv.DefaultLoad)
	if len(vars) > 0 {
		b.WriteString(renderVars(vars) + "\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (s *Session) cmdGoroutines() (string, error) {
	gs, _, err := s.c.ListGoroutines(0, 30)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, g := range gs {
		loc := g.UserCurrentLoc
		if loc.File == "" {
			loc = g.CurrentLoc
		}
		fn := "?"
		if loc.Function != nil {
			fn = loc.Function.Name
		}
		marker := "  "
		if s.selG == g.ID {
			marker = "* "
		}
		fmt.Fprintf(&b, "%sgoroutine %d  %s  %s:%d\n", marker, g.ID, fn, filepath.Base(loc.File), loc.Line)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (s *Session) cmdGoroutine(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("usage: gdbg goroutine <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return "", err
	}
	s.selG, s.selFrame = id, 0
	return s.cmdBt(nil)
}

func (s *Session) cmdFrame(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("usage: gdbg frame <n>|up|down")
	}
	switch args[0] {
	case "up":
		s.selFrame++
	case "down":
		if s.selFrame > 0 {
			s.selFrame--
		}
	default:
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 0 {
			return "", fmt.Errorf("usage: gdbg frame <n>|up|down")
		}
		s.selFrame = n
	}
	file, line, err := s.currentLocation()
	if err != nil {
		return "", err
	}
	head := fmt.Sprintf("frame %d selected\n", s.selFrame)
	return head + sourceContext(file, line, 3), nil
}
