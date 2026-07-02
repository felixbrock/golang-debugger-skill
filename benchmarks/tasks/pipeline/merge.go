package pipeline

import "sort"

// Merge joins directly adjacent windows of the same service when BOTH have
// ErrPerMin below MergeThreshold, to keep quiet periods as one span. The
// merged window spans both ranges; counts are summed and the rate is
// recomputed over the combined duration.
const MergeThreshold = 2.0

func Merge(windows []*Window) []*Window {
	sort.SliceStable(windows, func(i, j int) bool {
		if windows[i].Service != windows[j].Service {
			return windows[i].Service < windows[j].Service
		}
		return windows[i].Start < windows[j].Start
	})
	var out []*Window
	for _, w := range windows {
		if n := len(out); n > 0 {
			prev := out[n-1]
			if prev.Service == w.Service && prev.End == w.Start &&
				prev.ErrPerMin < MergeThreshold && w.ErrPerMin < MergeThreshold {
				prev.End = w.End
				prev.Events = append(prev.Events, w.Events...)
				prev.Errors += w.Errors
				minutes := float64(prev.End-prev.Start) / 60.0
				prev.ErrPerMin = float64(prev.Errors) / minutes
				continue
			}
		}
		out = append(out, w)
	}
	return out
}
