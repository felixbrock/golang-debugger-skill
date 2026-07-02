package pipeline

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse converts raw "ts|level|service|msg" lines into events. Blank lines
// and comment lines starting with '#' are skipped. Lines are not assumed to
// be sorted; ParseAll sorts by timestamp.
func Parse(line string) (Event, error) {
	parts := strings.SplitN(line, "|", 4)
	if len(parts) != 4 {
		return Event{}, fmt.Errorf("malformed line %q", line)
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return Event{}, fmt.Errorf("bad timestamp in %q: %v", line, err)
	}
	return Event{
		TS:      ts,
		Level:   strings.ToUpper(strings.TrimSpace(parts[1])),
		Service: strings.TrimSpace(parts[2]),
		Msg:     strings.TrimSpace(parts[3]),
	}, nil
}

// ParseAll parses every significant line, sorted by timestamp (stable for
// equal timestamps, preserving input order).
func ParseAll(raw string) ([]Event, error) {
	var evs []Event
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ev, err := Parse(line)
		if err != nil {
			return nil, err
		}
		evs = append(evs, ev)
	}
	// insertion sort keeps equal-TS events in input order
	for i := 1; i < len(evs); i++ {
		for j := i; j > 0 && evs[j-1].TS > evs[j].TS; j-- {
			evs[j-1], evs[j] = evs[j], evs[j-1]
		}
	}
	return evs, nil
}
