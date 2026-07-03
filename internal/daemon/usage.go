package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/felixbrock/golang-debugger-skill/internal/client"
)

// usageLog appends one JSONL record per executed command to
// .gdbg/usage.jsonl so feature usage can be analyzed later. Logging is
// best-effort: a missing or unwritable file never affects command handling.
type usageLog struct {
	mu sync.Mutex
	f  *os.File
}

type usageRecord struct {
	TS   string   `json:"ts"`
	Cmd  string   `json:"cmd"`
	Args []string `json:"args,omitempty"`
	OK   bool     `json:"ok"`
	Ms   int64    `json:"ms"`
	Err  string   `json:"err,omitempty"`
}

func openUsageLog(path string) *usageLog {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return &usageLog{}
	}
	return &usageLog{f: f}
}

func (u *usageLog) record(cmd string, args []string, start time.Time, err error) {
	if u == nil || u.f == nil {
		return
	}
	rec := usageRecord{
		TS:   start.UTC().Format(time.RFC3339),
		Cmd:  cmd,
		Args: args,
		OK:   err == nil,
		Ms:   time.Since(start).Milliseconds(),
	}
	if err != nil {
		rec.Err = err.Error()
	}
	line, jerr := json.Marshal(rec)
	if jerr != nil {
		return
	}
	u.mu.Lock()
	u.f.Write(append(line, '\n'))
	u.mu.Unlock()
}

// cmdUsage summarizes .gdbg/usage.jsonl: how often each command ran and
// how often it failed. `do` lines are containers; their subcommands are
// logged individually as well.
func (d *Daemon) cmdUsage() (string, error) {
	data, err := os.ReadFile(client.UsagePath(d.root))
	if err != nil {
		return "no usage recorded yet", nil
	}
	type stat struct {
		n, errs int
	}
	stats := map[string]*stat{}
	total, first := 0, ""
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var rec usageRecord
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if first == "" {
			first = rec.TS
		}
		total++
		s := stats[rec.Cmd]
		if s == nil {
			s = &stat{}
			stats[rec.Cmd] = s
		}
		s.n++
		if !rec.OK {
			s.errs++
		}
	}
	if total == 0 {
		return "no usage recorded yet", nil
	}
	cmds := make([]string, 0, len(stats))
	for c := range stats {
		cmds = append(cmds, c)
	}
	sort.Slice(cmds, func(i, j int) bool {
		if stats[cmds[i]].n != stats[cmds[j]].n {
			return stats[cmds[i]].n > stats[cmds[j]].n
		}
		return cmds[i] < cmds[j]
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%d commands since %s\n", total, first)
	for _, c := range cmds {
		s := stats[c]
		suffix := ""
		if s.errs > 0 {
			suffix = fmt.Sprintf("  (%d errors)", s.errs)
		}
		fmt.Fprintf(&b, "  %4d  %s%s\n", s.n, c, suffix)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
