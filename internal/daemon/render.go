package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/felixbrock/golang-debugger-skill/internal/dlv"
)

// ---------- stop reports ----------

func (s *Session) renderStop(state *dlv.DebuggerState, logLines []string, interrupted bool) string {
	var b strings.Builder
	for _, l := range logLines {
		b.WriteString(l + "\n")
	}

	th := state.CurrentThread
	bp := stopBreakpoint(state)
	reason := "step"
	switch {
	case interrupted:
		reason = "interrupted: still running after timeout, halted"
	case bp == nil:
	case bp.Name == "unrecovered-panic":
		reason = "panic"
	case bp.Name == "fatal-throw":
		reason = "fatal error"
	case bp.WatchExpr != "":
		reason = fmt.Sprintf("watchpoint %s", bp.WatchExpr)
	default:
		reason = "breakpoint"
	}

	loc, fn := "?", "?"
	var gid int64
	if th != nil {
		loc = fmt.Sprintf("%s:%d", filepath.Base(th.File), th.Line)
		if th.Function != nil {
			fn = th.Function.Name
		}
		gid = th.GoroutineID
	}
	fmt.Fprintf(&b, ">>> STOP [%s] %s  %s  (goroutine %d)\n", reason, fn, loc, gid)
	if th != nil {
		b.WriteString(sourceArrow(th.File, th.Line))
	}

	if reason == "panic" {
		s.renderPanic(&b)
	}
	if len(state.WatchOutOfScope) > 0 {
		for _, w := range state.WatchOutOfScope {
			fmt.Fprintf(&b, "note: watchpoint on %s went out of scope and was removed\n", w.WatchExpr)
		}
	}
	if len(th.ReturnValues) > 0 {
		for _, rv := range th.ReturnValues {
			fmt.Fprintf(&b, "return %s\n", inlineVar(rv, 100))
		}
	}

	s.renderDeltaLocals(&b)
	s.renderWatchExprs(&b)
	writeOutput(&b, s.proc.NewOutput())
	return strings.TrimRight(b.String(), "\n")
}

// renderPanic prints the panic value and jumps the view to the first
// non-runtime frame so vars/eval land where the panic was raised.
func (s *Session) renderPanic(b *strings.Builder) {
	if v, err := s.c.Eval(s.scope(), "runtime.curg._panic.arg", &dlv.DefaultLoad); err == nil {
		fmt.Fprintf(b, "panic: %s\n", inlineVar(*v, 200))
	}
	frames, err := s.c.Stacktrace(s.selG, 30, nil)
	if err != nil {
		return
	}
	for i, fr := range frames {
		name := ""
		if fr.Function != nil {
			name = fr.Function.Name
		}
		if !strings.HasPrefix(name, "runtime.") && name != "" {
			s.selFrame = i
			fmt.Fprintf(b, "frame %d selected: %s  %s:%d\n",
				i, name, filepath.Base(fr.File), fr.Line)
			b.WriteString(sourceArrow(fr.File, fr.Line))
			break
		}
	}
}

// renderDeltaLocals shows only locals that changed since the previous stop:
//	~ sum: int = 6 (was 3)
func (s *Session) renderDeltaLocals(b *strings.Builder) {
	vars := s.currentVars(dlv.DefaultLoad)
	next := map[string]string{}
	type change struct{ name, typ, now, was string }
	var changes []change
	for _, v := range vars {
		val := inlineVar(v, 90)
		next[v.Name] = val
		if old, ok := s.lastLocals[v.Name]; !ok || old != val {
			was := ""
			if ok {
				was = s.lastLocals[v.Name]
			}
			changes = append(changes, change{v.Name, v.Type, val, was})
		}
	}
	if len(changes) <= 12 {
		for _, c := range changes {
			if c.was != "" {
				fmt.Fprintf(b, "~ %s = %s (was %s)\n", c.name, c.now, c.was)
			} else {
				fmt.Fprintf(b, "+ %s = %s\n", c.name, c.now)
			}
		}
	} else {
		fmt.Fprintf(b, "(%d locals in scope; run `gdbg vars`)\n", len(changes))
	}
	s.lastLocals = next
}

func (s *Session) currentVars(cfg dlv.LoadConfig) []dlv.Variable {
	args, _ := s.c.ListFunctionArgs(s.scope(), cfg)
	locals, _ := s.c.ListLocalVars(s.scope(), cfg)
	return append(args, locals...)
}

func (s *Session) renderWatchExprs(b *strings.Builder) {
	for _, expr := range s.watchExprs {
		v, err := s.c.Eval(s.scope(), expr, &dlv.DefaultLoad)
		if err != nil {
			fmt.Fprintf(b, "watch %s = <error: %v>\n", expr, err)
			continue
		}
		fmt.Fprintf(b, "watch %s = %s\n", expr, inlineVar(*v, 100))
	}
}

func (s *Session) renderLogpoint(state *dlv.DebuggerState, tmpl string) string {
	th := state.CurrentThread
	msg := logpointRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		expr := m[1 : len(m)-1]
		v, err := s.c.Eval(dlv.EvalScope{GoroutineID: -1}, expr, &dlv.DefaultLoad)
		if err != nil {
			return "<err>"
		}
		return inlineVar(*v, 60)
	})
	loc := ""
	if th != nil {
		loc = fmt.Sprintf("%s:%d", filepath.Base(th.File), th.Line)
	}
	return fmt.Sprintf("log %s  %s", loc, msg)
}

var logpointRe = regexp.MustCompile(`\{[^{}]+\}`)

func writeOutput(b *strings.Builder, lines []string) {
	const max = 30
	if len(lines) > max {
		fmt.Fprintf(b, "out| … %d earlier output lines (gdbg output)\n", len(lines)-max)
		lines = lines[len(lines)-max:]
	}
	for _, l := range lines {
		b.WriteString("out| " + l + "\n")
	}
}

// sourceArrow renders "   ->    12 | <code>" for a file:line.
func sourceArrow(file string, line int) string {
	src := sourceLine(file, line)
	if src == "" {
		return ""
	}
	return fmt.Sprintf("   -> %5d | %s\n", line, src)
}

func sourceLine(file string, line int) string {
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return strings.TrimRight(lines[line-1], " \t")
}

// sourceContext renders a window of source around a line.
func sourceContext(file string, line, radius int) string {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Sprintf("cannot read %s: %v", file, err)
	}
	lines := strings.Split(string(data), "\n")
	var b strings.Builder
	for i := line - radius; i <= line+radius; i++ {
		if i < 1 || i > len(lines) {
			continue
		}
		marker := "      "
		if i == line {
			marker = "   -> "
		}
		fmt.Fprintf(&b, "%s%5d | %s\n", marker, i, strings.TrimRight(lines[i-1], " \t"))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---------- variable rendering ----------

// renderVars renders a variable list as an indented tree.
func renderVars(vars []dlv.Variable) string {
	var b strings.Builder
	for _, v := range vars {
		renderVarTree(&b, v, 1, v.Name)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderVarTree(b *strings.Builder, v dlv.Variable, depth int, label string) {
	indent := strings.Repeat("  ", depth)
	if label == "" {
		label = v.Name
	}
	if v.Unreadable != "" {
		fmt.Fprintf(b, "%s%s: %s = <unreadable: %s>\n", indent, label, v.Type, v.Unreadable)
		return
	}
	k := reflect.Kind(v.Kind)
	switch k {
	case reflect.Struct:
		if len(v.Children) == 0 {
			fmt.Fprintf(b, "%s%s: %s = %s\n", indent, label, v.Type, valueOrBrace(v))
			return
		}
		fmt.Fprintf(b, "%s%s: %s\n", indent, label, v.Type)
		for _, c := range v.Children {
			renderVarTree(b, c, depth+1, c.Name)
		}
	case reflect.Slice, reflect.Array:
		fmt.Fprintf(b, "%s%s: %s (len %d)\n", indent, label, v.Type, v.Len)
		for i, c := range v.Children {
			renderVarTree(b, c, depth+1, fmt.Sprintf("[%d]", i))
		}
		if int64(len(v.Children)) < v.Len {
			fmt.Fprintf(b, "%s  … %d more\n", indent, v.Len-int64(len(v.Children)))
		}
	case reflect.Map:
		fmt.Fprintf(b, "%s%s: %s (len %d)\n", indent, label, v.Type, v.Len)
		for i := 0; i+1 < len(v.Children); i += 2 {
			key := inlineVar(v.Children[i], 40)
			renderVarTree(b, v.Children[i+1], depth+1, "["+key+"]")
		}
	case reflect.Ptr:
		if len(v.Children) == 1 && !v.Children[0].OnlyAddr {
			renderVarTree(b, v.Children[0], depth, label+": "+v.Type+" ->")
			return
		}
		fmt.Fprintf(b, "%s%s: %s = %s\n", indent, label, v.Type, ptrValue(v))
	case reflect.Interface:
		if len(v.Children) == 1 {
			renderVarTree(b, v.Children[0], depth, label)
			return
		}
		fmt.Fprintf(b, "%s%s: %s = nil\n", indent, label, v.Type)
	case reflect.String:
		fmt.Fprintf(b, "%s%s: string = %s\n", indent, label, quoted(v))
	default:
		fmt.Fprintf(b, "%s%s: %s = %s\n", indent, label, v.Type, valueOrBrace(v))
	}
}

// inlineVar renders a variable on one line, capped at maxLen characters.
func inlineVar(v dlv.Variable, maxLen int) string {
	s := inlineValue(v, 3)
	if r := []rune(s); len(r) > maxLen {
		s = string(r[:maxLen-1]) + "…"
	}
	return s
}

func inlineValue(v dlv.Variable, depth int) string {
	if v.Unreadable != "" {
		return "<unreadable>"
	}
	if depth == 0 {
		return "…"
	}
	k := reflect.Kind(v.Kind)
	switch k {
	case reflect.String:
		return quoted(v)
	case reflect.Struct:
		if len(v.Children) == 0 {
			return valueOrBrace(v)
		}
		parts := make([]string, 0, len(v.Children))
		for _, c := range v.Children {
			parts = append(parts, c.Name+": "+inlineValue(c, depth-1))
		}
		return v.Type + "{" + strings.Join(parts, ", ") + "}"
	case reflect.Slice, reflect.Array:
		parts := make([]string, 0, len(v.Children))
		for _, c := range v.Children {
			parts = append(parts, inlineValue(c, depth-1))
		}
		ell := ""
		if int64(len(v.Children)) < v.Len {
			ell = ", …"
		}
		return "[" + strings.Join(parts, ", ") + ell + "]"
	case reflect.Map:
		var parts []string
		for i := 0; i+1 < len(v.Children); i += 2 {
			parts = append(parts, inlineValue(v.Children[i], depth-1)+": "+inlineValue(v.Children[i+1], depth-1))
		}
		return "map[" + strings.Join(parts, ", ") + "]"
	case reflect.Ptr:
		if len(v.Children) == 1 && !v.Children[0].OnlyAddr {
			return "&" + inlineValue(v.Children[0], depth)
		}
		return ptrValue(v)
	case reflect.Interface:
		if len(v.Children) == 1 {
			return inlineValue(v.Children[0], depth)
		}
		return "nil"
	default:
		return valueOrBrace(v)
	}
}

func quoted(v dlv.Variable) string {
	s := fmt.Sprintf("%q", v.Value)
	if v.Len > int64(len(v.Value)) {
		s += fmt.Sprintf("… (len %d)", v.Len)
	}
	return s
}

func ptrValue(v dlv.Variable) string {
	if v.Children == nil && v.Value == "" {
		return "nil"
	}
	if len(v.Children) == 1 && v.Children[0].OnlyAddr {
		if v.Children[0].Addr == 0 {
			return "nil"
		}
		return fmt.Sprintf("0x%x", v.Children[0].Addr)
	}
	if v.Value != "" {
		return v.Value
	}
	return "nil"
}

func valueOrBrace(v dlv.Variable) string {
	if v.Value != "" {
		return v.Value
	}
	if len(v.Children) == 0 {
		if reflect.Kind(v.Kind) == reflect.Func {
			return "nil"
		}
		return "{}"
	}
	return "{…}"
}
