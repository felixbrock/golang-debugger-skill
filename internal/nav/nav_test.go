package nav

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhere(t *testing.T) {
	dir := t.TempDir()
	src := `package p

type Config struct{ Threads int }

const MaxRetries = 3

func parseConfig(path string, strict bool) (*Config, error) { return nil, nil }

func (c *Config) Validate() error { return nil }
`
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := Where(dir, "parseConfig")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "p.go:7") || !strings.Contains(out, "func parseConfig(path, strict") &&
		!strings.Contains(out, "func parseConfig(path string, strict bool)") {
		t.Errorf("unexpected: %s", out)
	}

	out, _ = Where(dir, "Validate")
	if !strings.Contains(out, "(*Config)") {
		t.Errorf("method receiver missing: %s", out)
	}

	out, _ = Where(dir, "Config")
	if !strings.Contains(out, "type Config") {
		t.Errorf("type missing: %s", out)
	}

	out, _ = Where(dir, "MaxRetries")
	if !strings.Contains(out, "const MaxRetries") {
		t.Errorf("const missing: %s", out)
	}

	out, _ = Where(dir, "nothingHere")
	if !strings.Contains(out, "no declaration") {
		t.Errorf("expected no-result message: %s", out)
	}
}
