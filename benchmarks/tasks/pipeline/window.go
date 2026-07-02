package pipeline

// Windowize buckets events into fixed WindowSize windows per service, in
// timestamp order. Windows are created aligned to WindowSize boundaries.
func Windowize(evs []Event) []*Window {
	var windows []*Window
	byService := map[string][]*Window{}
	for _, ev := range evs {
		var target *Window
		for _, w := range byService[ev.Service] {
			if ev.TS >= w.Start && w.End >= ev.TS {
				target = w
				break
			}
		}
		if target == nil {
			start := ev.TS - ev.TS%WindowSize
			target = &Window{
				Service: ev.Service,
				Start:   start,
				End:     start + WindowSize,
			}
			byService[ev.Service] = append(byService[ev.Service], target)
			windows = append(windows, target)
		}
		target.Events = append(target.Events, ev)
		if ev.Level == "ERROR" {
			target.Errors++
		}
	}
	return windows
}
