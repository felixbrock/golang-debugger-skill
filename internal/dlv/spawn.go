package dlv

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Proc is a running headless dlv process plus the target's captured output.
type Proc struct {
	Cmd  *exec.Cmd
	Addr string

	mu     sync.Mutex
	output []string // ring buffer of target stdout+stderr lines
	seen   int      // lines already reported to the user
}

const outputRingSize = 400

var listenRe = regexp.MustCompile(`API server listening at: (\S+)`)

// Spawn starts `dlv <mode> ...` headless in dir and waits for the API server
// address. mode is "debug", "test" or "exec"; target is the package dir or
// binary path; targetArgs are passed to the program after "--". env, when
// non-nil, replaces the daemon's environment for dlv and the target.
func Spawn(dir, mode, target string, buildFlags string, targetArgs []string, env []string) (*Proc, error) {
	if _, err := exec.LookPath("dlv"); err != nil {
		return nil, fmt.Errorf("dlv not found on PATH; install it with: go install github.com/go-delve/delve/cmd/dlv@latest")
	}
	args := []string{mode, target,
		"--headless", "--api-version=2", "--accept-multiclient",
		"--listen=127.0.0.1:0"}
	if buildFlags != "" {
		args = append(args, "--build-flags="+buildFlags)
	}
	if len(targetArgs) > 0 {
		args = append(args, "--")
		args = append(args, targetArgs...)
	}
	cmd := exec.Command("dlv", args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	cmd.SysProcAttr = sysProcAttr()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout // interleave; dlv sends target output to both
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	p := &Proc{Cmd: cmd}
	addrCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		gotAddr := false
		var pre []string
		for sc.Scan() {
			line := sc.Text()
			if !gotAddr {
				if m := listenRe.FindStringSubmatch(line); m != nil {
					gotAddr = true
					addrCh <- m[1]
					continue
				}
				pre = append(pre, line)
				continue
			}
			p.appendOutput(line)
		}
		if !gotAddr {
			errCh <- fmt.Errorf("dlv exited before serving:\n%s", strings.Join(pre, "\n"))
		}
	}()

	select {
	case addr := <-addrCh:
		p.Addr = addr
		return p, nil
	case err := <-errCh:
		cmd.Wait()
		return nil, err
	case <-time.After(120 * time.Second):
		p.Kill()
		return nil, fmt.Errorf("timed out waiting for dlv to start")
	}
}

func (p *Proc) appendOutput(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.HasPrefix(line, "API server listening at:") ||
		strings.HasPrefix(line, "debugserver or lldb-server") {
		return
	}
	p.output = append(p.output, line)
	if len(p.output) > outputRingSize {
		drop := len(p.output) - outputRingSize
		p.output = p.output[drop:]
		p.seen -= drop
		if p.seen < 0 {
			p.seen = 0
		}
	}
}

// NewOutput returns target output lines produced since the last call.
func (p *Proc) NewOutput() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]string(nil), p.output[p.seen:]...)
	p.seen = len(p.output)
	return out
}

// AllOutput returns the full captured ring buffer.
func (p *Proc) AllOutput() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = len(p.output)
	return append([]string(nil), p.output...)
}

// Kill terminates dlv and the target process group.
func (p *Proc) Kill() {
	if p.Cmd.Process != nil {
		syscall.Kill(-p.Cmd.Process.Pid, syscall.SIGKILL)
		p.Cmd.Process.Kill()
	}
	go p.Cmd.Wait()
}
