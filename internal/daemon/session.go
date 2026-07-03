package daemon

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/felixbrock/golang-debugger-skill/internal/dlv"
)

// resumeTimeout bounds how long continue/step block before gdbg interrupts
// the target and reports where it is. Keeps every agent call one turn.
const resumeTimeout = 25 * time.Second

// Session is one live debug target: a headless dlv process plus view state.
type Session struct {
	mu   sync.Mutex
	proc *dlv.Proc
	c    *dlv.Client

	// launch config, kept for restart/reporting
	mode       string // debug | test | exec
	target     string
	targetArgs []string

	// view state
	selG       int64 // goroutine id; -1 = current
	selFrame   int
	watchExprs []string
	logpoints  map[int]string    // breakpoint id -> message template
	lastLocals map[string]string // for delta stops
	exited     bool
}

func newSession(root, mode, target, buildFlags string, targetArgs []string, env []string) (*Session, error) {
	proc, err := dlv.Spawn(root, mode, target, buildFlags, targetArgs, env)
	if err != nil {
		return nil, err
	}
	c, err := dlv.Dial(proc.Addr)
	if err != nil {
		out := strings.Join(proc.AllOutput(), "\n")
		proc.Kill()
		if out != "" {
			return nil, fmt.Errorf("%w\ndlv output:\n%s", err, out)
		}
		return nil, err
	}
	return &Session{
		proc: proc, c: c,
		mode: mode, target: target, targetArgs: targetArgs,
		selG: -1, logpoints: map[int]string{}, lastLocals: map[string]string{},
	}, nil
}

func (s *Session) scope() dlv.EvalScope {
	return dlv.EvalScope{GoroutineID: s.selG, Frame: s.selFrame}
}

func (s *Session) close() {
	if s.c != nil {
		s.c.Detach(true)
		s.c.Close()
	}
	if s.proc != nil {
		s.proc.Kill()
	}
}

// runCommand executes a resume command, interrupting the target if it does
// not stop within resumeTimeout. Returns the state and whether we halted it.
func (s *Session) runCommand(name string) (*dlv.DebuggerState, bool, error) {
	type res struct {
		st  *dlv.DebuggerState
		err error
	}
	ch := make(chan res, 1)
	go func() {
		st, err := s.c.Command(dlv.DebuggerCommand{Name: name})
		ch <- res{st, err}
	}()
	select {
	case r := <-ch:
		return r.st, false, r.err
	case <-time.After(resumeTimeout):
		// Concurrent call on the same connection; delve processes halt
		// while the resume is blocked.
		go s.c.Command(dlv.DebuggerCommand{Name: dlv.Halt})
		select {
		case r := <-ch:
			return r.st, true, r.err
		case <-time.After(10 * time.Second):
			return nil, true, fmt.Errorf("target did not stop after interrupt")
		}
	}
}

// resume continues (or steps) and produces the stop report, transparently
// running logpoints (print-and-continue breakpoints).
func (s *Session) resume(cmdName string) (string, error) {
	if s.exited {
		return "", fmt.Errorf("program has exited; use restart or launch")
	}
	var logLines []string
	for {
		state, interrupted, err := s.runCommand(cmdName)
		if err != nil {
			if strings.Contains(err.Error(), "has exited") {
				s.exited = true
				return s.exitReport(logLines, err.Error()), nil
			}
			return "", err
		}
		if state.Exited {
			s.exited = true
			return s.exitReport(logLines,
				fmt.Sprintf("process exited with status %d", state.ExitStatus)), nil
		}
		if bp := stopBreakpoint(state); bp != nil {
			if tmpl, ok := s.logpoints[bp.ID]; ok {
				logLines = append(logLines, s.renderLogpoint(state, tmpl))
				cmdName = dlv.Continue
				continue
			}
		}
		// Reset the view to where we stopped.
		s.selG, s.selFrame = -1, 0
		return s.renderStop(state, logLines, interrupted), nil
	}
}

func stopBreakpoint(state *dlv.DebuggerState) *dlv.Breakpoint {
	if state.CurrentThread != nil {
		return state.CurrentThread.Breakpoint
	}
	return nil
}

func (s *Session) exitReport(logLines []string, msg string) string {
	var b strings.Builder
	for _, l := range logLines {
		b.WriteString(l + "\n")
	}
	b.WriteString(">>> EXIT " + msg + "\n")
	s.reportNeverHit(&b)
	writeOutput(&b, s.proc.NewOutput())
	return strings.TrimRight(b.String(), "\n")
}

// reportNeverHit lists user breakpoints that never fired during the run.
// Silence here is dangerous: an agent that set a breakpoint and saw the
// program exit without stopping tends to conclude "this code is never
// reached", which is wrong when the location or condition was off.
func (s *Session) reportNeverHit(b *strings.Builder) {
	bps, err := s.c.ListBreakpoints()
	if err != nil {
		return
	}
	for _, bp := range bps {
		if bp.ID <= 0 || bp.TotalHitCount > 0 || bp.WatchExpr != "" {
			continue
		}
		loc := fmt.Sprintf("%s:%d", filepath.Base(bp.File), bp.Line)
		if bp.FunctionName != "" {
			loc += " in " + bp.FunctionName
		}
		extra := ""
		if bp.Cond != "" {
			extra = fmt.Sprintf(" (condition %q never true)", bp.Cond)
		}
		if bp.Disabled {
			extra = " (disabled)"
		}
		fmt.Fprintf(b, "note: breakpoint %d at %s was NEVER HIT%s — the line was bound but not executed; verify the location before concluding this code path is unreached\n",
			bp.ID, loc, extra)
	}
}
