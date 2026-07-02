package daemon

import (
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	flags, pos, rest, err := parseArgs(
		[]string{"--pkg", ".", "--break", "a.go:1", "--break=b.go:2", "--stop-entry", "x", "--", "-run", "TestX"},
		launchSpec)
	if err != nil {
		t.Fatal(err)
	}
	if first(flags, "pkg") != "." {
		t.Errorf("pkg = %q", first(flags, "pkg"))
	}
	if !reflect.DeepEqual(flags["break"], []string{"a.go:1", "b.go:2"}) {
		t.Errorf("break = %v", flags["break"])
	}
	if !has(flags, "stop-entry") {
		t.Error("missing stop-entry")
	}
	if !reflect.DeepEqual(pos, []string{"x"}) {
		t.Errorf("pos = %v", pos)
	}
	if !reflect.DeepEqual(rest, []string{"-run", "TestX"}) {
		t.Errorf("rest = %v", rest)
	}
}

func TestParseArgsUnknownFlag(t *testing.T) {
	if _, _, _, err := parseArgs([]string{"--bogus"}, launchSpec); err == nil {
		t.Error("expected error for unknown flag")
	}
}

func TestNormalizeTestArgs(t *testing.T) {
	got := normalizeTestArgs("test", []string{"-run", "TestX", "-v", "-count=1", "positional"})
	want := []string{"-test.run", "TestX", "-test.v", "-test.count=1", "positional"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	// non-test mode untouched
	got = normalizeTestArgs("debug", []string{"-run", "x"})
	if !reflect.DeepEqual(got, []string{"-run", "x"}) {
		t.Errorf("got %v", got)
	}
}

func TestLaunchModeExclusive(t *testing.T) {
	if _, _, err := launchMode(map[string][]string{"pkg": {"."}, "test": {"."}}); err == nil {
		t.Error("expected error for two modes")
	}
	mode, target, err := launchMode(map[string][]string{"bin-path": {"./app"}})
	if err != nil || mode != "exec" || target != "./app" {
		t.Errorf("got %s %s %v", mode, target, err)
	}
}
