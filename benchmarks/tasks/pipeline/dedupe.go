package pipeline

// Dedupe drops repeated events inside each window: an event is a duplicate
// if an event with the same level and message occurred at most DupeGap
// seconds before it in the same window. Error counts are recomputed.
const DupeGap = 5

func Dedupe(windows []*Window) {
	for _, w := range windows {
		var kept []Event
		lastSeen := map[string]int64{}
		w.Errors = 0
		for _, ev := range w.Events {
			key := ev.Level + "\x00" + ev.Msg
			if prev, ok := lastSeen[key]; ok && ev.TS-prev <= DupeGap {
				lastSeen[key] = ev.TS
				continue
			}
			lastSeen[key] = ev.TS
			kept = append(kept, ev)
			if ev.Level == "ERROR" {
				w.Errors++
			}
		}
		w.Events = kept
	}
}
