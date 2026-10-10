package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func runCLI(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "targets.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func decodeOutput(t *testing.T, raw string) jsonOutput {
	t.Helper()
	var doc jsonOutput
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, raw)
	}
	return doc
}

func TestSingleTargetOpen(t *testing.T) {
	addr := startListener(t).Addr().String()
	code, out, errOut := runCLI(addr)
	if code != exitOK {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	if !strings.Contains(out, addr) || !strings.Contains(out, "open") {
		t.Errorf("stdout = %q", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}
}

func TestSingleTargetRefused(t *testing.T) {
	addr := closedAddress(t)
	code, out, _ := runCLI("--timeout", "5s", addr)
	if code != exitFailed {
		t.Fatalf("exit = %d, want 1\nstdout: %s", code, out)
	}
	if !strings.Contains(out, "refused") {
		t.Errorf("stdout = %q, want it to mention refused", out)
	}
}

func TestSingleTargetDNSFailure(t *testing.T) {
	code, out, _ := runCLI("--timeout", "5s", "portpeek-test.invalid:80")
	if code != exitFailed {
		t.Fatalf("exit = %d, want 1\nstdout: %s", code, out)
	}
	if !strings.Contains(out, "dns_error") {
		t.Errorf("stdout = %q, want it to mention dns_error", out)
	}
}

func TestArgumentOrderIsFlexible(t *testing.T) {
	addr := startListener(t).Addr().String()
	variants := [][]string{
		{"--json", "--timeout", "2s", addr},
		{addr, "--timeout=2s", "--json"},
		{"--timeout=2s", addr, "--json"},
		{"--json", "--", addr},
	}
	for _, args := range variants {
		code, out, errOut := runCLI(args...)
		if code != exitOK {
			t.Errorf("args %v: exit = %d, stderr = %s", args, code, errOut)
			continue
		}
		doc := decodeOutput(t, out)
		if len(doc.Results) != 1 || doc.Results[0].Address != addr {
			t.Errorf("args %v: unexpected output %s", args, out)
		}
	}
}

func TestJSONSingleOpen(t *testing.T) {
	addr := startListener(t).Addr().String()
	code, out, errOut := runCLI("--json", addr)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	doc := decodeOutput(t, out)
	if len(doc.Results) != 1 {
		t.Fatalf("got %d results", len(doc.Results))
	}
	r := doc.Results[0]
	if r.Status != statusOpen || r.Error != nil || r.Address != addr || r.Name != "" {
		t.Errorf("unexpected result: %+v", r)
	}
	if doc.Summary != (jsonSummary{Total: 1, Open: 1, Failed: 0}) {
		t.Errorf("summary = %+v", doc.Summary)
	}
}

func TestJSONSingleRefused(t *testing.T) {
	code, out, _ := runCLI("--json", "--timeout", "5s", closedAddress(t))
	if code != exitFailed {
		t.Fatalf("exit = %d, want 1", code)
	}
	doc := decodeOutput(t, out)
	r := doc.Results[0]
	if r.Status != statusRefused || r.Error == nil || *r.Error != "connection refused" {
		t.Errorf("unexpected result: %+v", r)
	}
	if doc.Summary != (jsonSummary{Total: 1, Open: 0, Failed: 1}) {
		t.Errorf("summary = %+v", doc.Summary)
	}
}

func TestNoArguments(t *testing.T) {
	code, out, errOut := runCLI()
	if code != exitError {
		t.Fatalf("exit = %d, want 2", code)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	if !strings.Contains(errOut, "Usage:") {
		t.Errorf("stderr = %q, want usage text", errOut)
	}
}

func TestUnknownOption(t *testing.T) {
	code, out, errOut := runCLI("--bogus", "localhost:80")
	if code != exitError || out != "" {
		t.Fatalf("exit = %d, stdout = %q", code, out)
	}
	if !strings.Contains(errOut, `unknown option "--bogus"`) || !strings.Contains(errOut, "--help") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestInvalidTimeoutValues(t *testing.T) {
	for _, v := range []string{"abc", "0", "0s", "-1s", "3", ""} {
		code, out, errOut := runCLI("--timeout", v, "localhost:80")
		if code != exitError || out != "" {
			t.Errorf("--timeout %q: exit = %d, stdout = %q", v, code, out)
		}
		if !strings.Contains(errOut, "invalid timeout") {
			t.Errorf("--timeout %q: stderr = %q", v, errOut)
		}
	}
	code, _, errOut := runCLI("localhost:80", "--timeout")
	if code != exitError || !strings.Contains(errOut, "--timeout requires a value") {
		t.Errorf("missing value: exit = %d, stderr = %q", code, errOut)
	}
	code, _, errOut = runCLI("--timeout=bad", "localhost:80")
	if code != exitError || !strings.Contains(errOut, "invalid timeout") {
		t.Errorf("--timeout=bad: exit = %d, stderr = %q", code, errOut)
	}
}

func TestVersion(t *testing.T) {
	code, out, errOut := runCLI("--version")
	if code != exitOK || errOut != "" {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if out != "portpeek "+version+"\n" {
		t.Errorf("stdout = %q", out)
	}
}

func TestHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		code, out, errOut := runCLI(flag)
		if code != exitOK || errOut != "" {
			t.Fatalf("%s: exit = %d, stderr = %q", flag, code, errOut)
		}
		for _, want := range []string{"Usage:", "--timeout", "--json", "--version", "Exit codes:"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: help text missing %q", flag, want)
			}
		}
	}
}

func TestInvalidAddress(t *testing.T) {
	for _, addr := range []string{"localhost", "localhost:0", ":80", "http://localhost:80", "localhost:abc"} {
		code, out, errOut := runCLI("--json", addr)
		if code != exitError {
			t.Errorf("%q: exit = %d, want 2", addr, code)
		}
		if out != "" {
			t.Errorf("%q: stdout = %q, want empty", addr, out)
		}
		if !strings.Contains(errOut, "invalid address") {
			t.Errorf("%q: stderr = %q", addr, errOut)
		}
	}
}

func TestTooManyArguments(t *testing.T) {
	code, out, errOut := runCLI("localhost:80", "localhost:81")
	if code != exitError || out != "" {
		t.Fatalf("exit = %d, stdout = %q", code, out)
	}
	if !strings.Contains(errOut, "expected a single <host:port>") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestCheckArgumentErrors(t *testing.T) {
	cfgPath := writeConfigFile(t, "targets:\n  - address: localhost:1\n")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no path", []string{"check"}, "requires exactly one config file"},
		{"extra path", []string{"check", cfgPath, cfgPath}, "requires exactly one config file"},
		{"missing file", []string{"check", filepath.Join(t.TempDir(), "nope.yaml")}, "cannot read config file"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out, errOut := runCLI(append([]string{"--json"}, c.args...)...)
			if code != exitError || out != "" {
				t.Fatalf("exit = %d, stdout = %q", code, out)
			}
			if !strings.Contains(errOut, c.want) {
				t.Errorf("stderr = %q, want %q", errOut, c.want)
			}
		})
	}
}

func TestCheckInvalidConfigExitsWithTwo(t *testing.T) {
	path := writeConfigFile(t, "targets:\n  - name: A\n    address: nonsense\n")
	code, out, errOut := runCLI("--json", "check", path)
	if code != exitError || out != "" {
		t.Fatalf("exit = %d, stdout = %q", code, out)
	}
	if !strings.Contains(errOut, "invalid address") || !strings.Contains(errOut, ":2:") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestCheckHumanOutputOrderAndExitCode(t *testing.T) {
	a1 := startListener(t).Addr().String()
	a2 := startListener(t).Addr().String()
	closed := closedAddress(t)
	path := writeConfigFile(t, fmt.Sprintf(
		"targets:\n  - name: First\n    address: %s\n  - name: Second\n    address: %s\n  - name: Third\n    address: %s\ntimeout: 5s\n",
		a1, closed, a2))

	code, out, errOut := runCLI("check", path)
	if code != exitFailed {
		t.Fatalf("exit = %d, want 1\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines:\n%s", len(lines), out)
	}
	wants := []string{"First", "Second", "Third"}
	for i, w := range wants {
		if !strings.Contains(lines[i], w) {
			t.Errorf("line %d = %q, want it to contain %q", i, lines[i], w)
		}
	}
	if !strings.Contains(lines[1], "refused") {
		t.Errorf("line 1 = %q, want refused", lines[1])
	}
	if lines[3] != "3 checked: 2 open, 1 failed" {
		t.Errorf("summary line = %q", lines[3])
	}
}

func TestCheckAllOpenExitsZero(t *testing.T) {
	a1 := startListener(t).Addr().String()
	a2 := startListener(t).Addr().String()
	path := writeConfigFile(t, fmt.Sprintf("targets:\n  - address: %s\n  - address: %s\n", a1, a2))
	code, out, errOut := runCLI("check", path)
	if code != exitOK {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
}

func TestCheckJSONOrderAndSchema(t *testing.T) {
	var addrs []string
	var cfg strings.Builder
	cfg.WriteString("targets:\n")
	for i := 0; i < 6; i++ {
		var addr string
		if i%2 == 0 {
			addr = startListener(t).Addr().String()
		} else {
			addr = closedAddress(t)
		}
		addrs = append(addrs, addr)
		fmt.Fprintf(&cfg, "  - name: T%d\n    address: %s\n", i, addr)
	}
	cfg.WriteString("timeout: 5s\n")
	path := writeConfigFile(t, cfg.String())

	code, out, errOut := runCLI("--json", "check", path)
	if code != exitFailed {
		t.Fatalf("exit = %d, want 1\nstderr: %s", code, errOut)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	doc := decodeOutput(t, out)
	if len(doc.Results) != len(addrs) {
		t.Fatalf("got %d results", len(doc.Results))
	}
	for i, r := range doc.Results {
		if r.Address != addrs[i] || r.Name != fmt.Sprintf("T%d", i) {
			t.Errorf("result %d = %+v, want address %s", i, r, addrs[i])
		}
		wantStatus := statusOpen
		if i%2 == 1 {
			wantStatus = statusRefused
		}
		if r.Status != wantStatus {
			t.Errorf("result %d status = %q, want %q", i, r.Status, wantStatus)
		}
		if (r.Status == statusOpen) != (r.Error == nil) {
			t.Errorf("result %d: status %q with error %v", i, r.Status, r.Error)
		}
	}
	if doc.Summary != (jsonSummary{Total: 6, Open: 3, Failed: 3}) {
		t.Errorf("summary = %+v", doc.Summary)
	}
}

func TestJSONSchemaIdenticalForSingleAndCheck(t *testing.T) {
	open := startListener(t).Addr().String()
	closed := closedAddress(t)
	path := writeConfigFile(t, fmt.Sprintf("targets:\n  - name: A\n    address: %s\n  - address: %s\n", open, closed))

	keysOf := func(raw string) [][]string {
		var doc struct {
			Results []map[string]any `json:"results"`
		}
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatal(err)
		}
		var all [][]string
		for _, r := range doc.Results {
			var keys []string
			for k := range r {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			all = append(all, keys)
		}
		return all
	}

	_, single, _ := runCLI("--json", open)
	_, multi, _ := runCLI("--timeout", "5s", "--json", "check", path)
	want := "address,error,latency_ms,name,status"
	for _, raw := range []string{single, multi} {
		for _, keys := range keysOf(raw) {
			if got := strings.Join(keys, ","); got != want {
				t.Errorf("keys = %s, want %s", got, want)
			}
		}
	}
}

func TestPickTimeout(t *testing.T) {
	cfg := Config{Timeout: 3 * time.Second}
	flagged := options{timeout: 7 * time.Second, timeoutSet: true}
	if got := pickTimeout(flagged, cfg); got != 7*time.Second {
		t.Errorf("flag should win: got %v", got)
	}
	if got := pickTimeout(options{}, cfg); got != 3*time.Second {
		t.Errorf("config should apply: got %v", got)
	}
	if got := pickTimeout(options{}, Config{}); got != defaultTimeout {
		t.Errorf("default should apply: got %v", got)
	}
}

func TestParseArgs(t *testing.T) {
	o, err := parseArgs([]string{"--json", "--timeout=250ms", "check", "f.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.json || !o.timeoutSet || o.timeout != 250*time.Millisecond {
		t.Errorf("options = %+v", o)
	}
	if strings.Join(o.args, " ") != "check f.yaml" {
		t.Errorf("args = %v", o.args)
	}
	if _, err := parseArgs([]string{"-x"}); err == nil {
		t.Error("expected error for unknown short option")
	}
	o, err = parseArgs([]string{"-"})
	if err != nil || len(o.args) != 1 || o.args[0] != "-" {
		t.Errorf("single dash should be positional: %+v, %v", o, err)
	}
}
