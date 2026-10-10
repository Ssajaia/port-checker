package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseConfigSpecExample(t *testing.T) {
	input := `targets:
  - name: Local API
    address: localhost:8080
  - name: HTTPS
    address: example.com:443
  - name: SSH
    address: example.com:22
timeout: 3s
`
	cfg, err := parseConfig("targets.yaml", []byte(input))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	want := []Target{
		{Name: "Local API", Address: "localhost:8080"},
		{Name: "HTTPS", Address: "example.com:443"},
		{Name: "SSH", Address: "example.com:22"},
	}
	if len(cfg.Targets) != len(want) {
		t.Fatalf("got %d targets, want %d", len(cfg.Targets), len(want))
	}
	for i := range want {
		if cfg.Targets[i] != want[i] {
			t.Errorf("target %d = %+v, want %+v", i, cfg.Targets[i], want[i])
		}
	}
	if cfg.Timeout != 3*time.Second {
		t.Errorf("timeout = %v, want 3s", cfg.Timeout)
	}
}

func TestParseConfigValidVariants(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantTargets []Target
		wantTimeout time.Duration
	}{
		{
			name:        "no timeout",
			input:       "targets:\n  - address: a.example:1\n",
			wantTargets: []Target{{Address: "a.example:1"}},
		},
		{
			name:        "timeout before targets",
			input:       "timeout: 250ms\ntargets:\n  - address: a.example:1\n",
			wantTargets: []Target{{Address: "a.example:1"}},
			wantTimeout: 250 * time.Millisecond,
		},
		{
			name:        "unindented list items",
			input:       "targets:\n- name: A\n  address: a.example:1\n- address: b.example:2\n",
			wantTargets: []Target{{Name: "A", Address: "a.example:1"}, {Address: "b.example:2"}},
		},
		{
			name:        "dash on its own line",
			input:       "targets:\n  -\n    name: A\n    address: a.example:1\n",
			wantTargets: []Target{{Name: "A", Address: "a.example:1"}},
		},
		{
			name:        "address first then name",
			input:       "targets:\n  - address: a.example:1\n    name: A\n",
			wantTargets: []Target{{Name: "A", Address: "a.example:1"}},
		},
		{
			name:        "crlf line endings",
			input:       "targets:\r\n  - name: A\r\n    address: a.example:1\r\ntimeout: 2s\r\n",
			wantTargets: []Target{{Name: "A", Address: "a.example:1"}},
			wantTimeout: 2 * time.Second,
		},
		{
			name:        "utf-8 byte order mark",
			input:       "\xef\xbb\xbftargets:\n  - address: a.example:1\n",
			wantTargets: []Target{{Address: "a.example:1"}},
		},
		{
			name:        "comments and blank lines",
			input:       "# header\n\ntargets:\n  # first\n  - name: A\n\n    address: a.example:1\n\n# done\n",
			wantTargets: []Target{{Name: "A", Address: "a.example:1"}},
		},
		{
			name:        "quoted values",
			input:       "targets:\n  - name: \"Quoted: name\"\n    address: 'a.example:1'\ntimeout: \"4s\"\n",
			wantTargets: []Target{{Name: "Quoted: name", Address: "a.example:1"}},
			wantTimeout: 4 * time.Second,
		},
		{
			name:        "ipv6 address",
			input:       "targets:\n  - address: \"[::1]:8080\"\n",
			wantTargets: []Target{{Address: "[::1]:8080"}},
		},
		{
			name:        "no trailing newline",
			input:       "targets:\n  - address: a.example:1",
			wantTargets: []Target{{Address: "a.example:1"}},
		},
		{
			name:        "same name check on different addresses with empty names",
			input:       "targets:\n  - address: a.example:1\n  - address: a.example:2\n",
			wantTargets: []Target{{Address: "a.example:1"}, {Address: "a.example:2"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := parseConfig("c.yaml", []byte(c.input))
			if err != nil {
				t.Fatalf("parseConfig: %v", err)
			}
			if len(cfg.Targets) != len(c.wantTargets) {
				t.Fatalf("got %d targets, want %d", len(cfg.Targets), len(c.wantTargets))
			}
			for i := range c.wantTargets {
				if cfg.Targets[i] != c.wantTargets[i] {
					t.Errorf("target %d = %+v, want %+v", i, cfg.Targets[i], c.wantTargets[i])
				}
			}
			if cfg.Timeout != c.wantTimeout {
				t.Errorf("timeout = %v, want %v", cfg.Timeout, c.wantTimeout)
			}
		})
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty file", "", "'targets' is required"},
		{"only timeout", "timeout: 3s\n", "'targets' is required"},
		{"empty targets", "targets:\n", "'targets' is required"},
		{"targets with inline value", "targets: foo\n", "must be followed by a list"},
		{"missing address", "targets:\n  - name: A\n", "c.yaml:2: target #1 is missing required field 'address'"},
		{"invalid address", "targets:\n  - address: localhost\n", "c.yaml:2: target #1 has invalid address"},
		{"invalid port", "targets:\n  - address: localhost:99999\n", "invalid address"},
		{"address with space from inline comment", "targets:\n  - address: a.example:1 # note\n", "invalid address"},
		{"bad timeout", "targets:\n  - address: a.example:1\ntimeout: soon\n", "c.yaml:3: invalid timeout"},
		{"missing timeout unit", "targets:\n  - address: a.example:1\ntimeout: 3\n", "invalid timeout"},
		{"empty timeout", "targets:\n  - address: a.example:1\ntimeout:\n", "invalid timeout"},
		{"zero timeout", "targets:\n  - address: a.example:1\ntimeout: 0s\n", "greater than zero"},
		{"negative timeout", "targets:\n  - address: a.example:1\ntimeout: -5s\n", "greater than zero"},
		{"duplicate name", "targets:\n  - name: A\n    address: a.example:1\n  - name: A\n    address: b.example:2\n", "c.yaml:4: target #2 reuses name \"A\""},
		{"duplicate address", "targets:\n  - name: A\n    address: a.example:1\n  - name: B\n    address: a.example:1\n", "repeats address"},
		{"duplicate timeout", "timeout: 1s\ntimeout: 2s\ntargets:\n  - address: a.example:1\n", "duplicate key 'timeout'"},
		{"duplicate targets", "targets:\n  - address: a.example:1\ntargets:\n  - address: b.example:2\n", "duplicate key 'targets'"},
		{"unknown top-level key", "retries: 3\ntargets:\n  - address: a.example:1\n", "unknown key \"retries\""},
		{"unknown target field", "targets:\n  - addr: a.example:1\n", "unknown field \"addr\""},
		{"duplicate target field", "targets:\n  - address: a.example:1\n    address: b.example:2\n", "duplicate field \"address\""},
		{"tab indentation", "targets:\n\t- address: a.example:1\n", "tabs are not allowed"},
		{"indented without targets", "  foo: bar\n", "unexpected indented line"},
		{"indented after timeout", "targets:\n  - address: a.example:1\ntimeout: 1s\n  - address: b.example:2\n", "unexpected indented line"},
		{"field before list item", "targets:\n    address: a.example:1\n", "expected a target starting with '- '"},
		{"malformed target line", "targets:\n  - address\n", "expected 'key: value'"},
		{"malformed top-level line", "hello\n", "expected 'key: value'"},
		{"missing space after colon", "targets:\n  - address:a.example:1\n", "expected 'key: value'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseConfig("c.yaml", []byte(c.input))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "targets.yaml")
	content := "targets:\n  - name: A\n    address: a.example:1\ntimeout: 1500ms\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(cfg.Targets) != 1 || cfg.Timeout != 1500*time.Millisecond {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := loadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil || !strings.Contains(err.Error(), "cannot read config file") {
		t.Errorf("error = %v, want cannot read config file", err)
	}
}

func TestLoadConfigErrorIncludesFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("targets:\n  - name: A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), path+":2:") {
		t.Errorf("error = %v, want it to start with %q", err, path+":2:")
	}
}

func TestParseTimeout(t *testing.T) {
	good := map[string]time.Duration{
		"500ms": 500 * time.Millisecond,
		"3s":    3 * time.Second,
		"1m":    time.Minute,
		"1.5s":  1500 * time.Millisecond,
	}
	for in, want := range good {
		got, err := parseTimeout(in)
		if err != nil || got != want {
			t.Errorf("parseTimeout(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "3", "0", "0s", "-1s", "1x"} {
		if _, err := parseTimeout(in); err == nil {
			t.Errorf("parseTimeout(%q) returned nil error", in)
		}
	}
}
