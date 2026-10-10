package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFormatLatency(t *testing.T) {
	cases := map[time.Duration]string{
		0:                       "0.0ms",
		1500 * time.Microsecond: "1.5ms",
		250 * time.Millisecond:  "250.0ms",
		1500 * time.Millisecond: "1.50s",
		3 * time.Second:         "3.00s",
	}
	for d, want := range cases {
		if got := formatLatency(d); got != want {
			t.Errorf("formatLatency(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestWriteHumanSingleUnnamed(t *testing.T) {
	var buf bytes.Buffer
	err := writeHuman(&buf, []Result{{
		Target:  Target{Address: "localhost:8080"},
		Status:  statusOpen,
		Latency: 12 * time.Millisecond,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "open  localhost:8080  12.0ms\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestWriteHumanNamedWithFailuresAndSummary(t *testing.T) {
	results := []Result{
		{Target: Target{Name: "API", Address: "localhost:8080"}, Status: statusOpen, Latency: 2 * time.Millisecond},
		{Target: Target{Address: "example.com:22"}, Status: statusRefused, Latency: 3 * time.Millisecond, Err: "connection refused"},
		{Target: Target{Name: "Slow", Address: "example.com:443"}, Status: statusTimeout, Latency: 3 * time.Second, Err: "no response within 3s"},
	}
	var buf bytes.Buffer
	if err := writeHuman(&buf, results); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), buf.String())
	}
	checks := [][]string{
		{"open", "API", "localhost:8080", "2.0ms"},
		{"refused", "-", "example.com:22", "3.0ms", "connection refused"},
		{"timeout", "Slow", "example.com:443", "3.00s", "no response within 3s"},
		{"3 checked: 1 open, 2 failed"},
	}
	for i, parts := range checks {
		for _, p := range parts {
			if !strings.Contains(lines[i], p) {
				t.Errorf("line %d = %q, missing %q", i, lines[i], p)
			}
		}
		if lines[i] != strings.TrimRight(lines[i], " ") {
			t.Errorf("line %d has trailing spaces: %q", i, lines[i])
		}
	}
	col := strings.Index(lines[0], "localhost:8080")
	if strings.Index(lines[1], "example.com:22") != col || strings.Index(lines[2], "example.com:443") != col {
		t.Errorf("address column is not aligned:\n%s", buf.String())
	}
}

func TestWriteHumanEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeHuman(&buf, nil); err != nil || buf.Len() != 0 {
		t.Errorf("empty results: err=%v output=%q", err, buf.String())
	}
}

func TestWriteJSONSchema(t *testing.T) {
	results := []Result{
		{Target: Target{Name: "API", Address: "localhost:8080"}, Status: statusOpen, Latency: 1234567 * time.Nanosecond},
		{Target: Target{Address: "example.com:22"}, Status: statusRefused, Latency: time.Millisecond, Err: "connection refused"},
	}
	var buf bytes.Buffer
	if err := writeJSON(&buf, results); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("invalid JSON: %s", buf.String())
	}

	var doc struct {
		Results []map[string]any `json:"results"`
		Summary map[string]any   `json:"summary"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Results) != 2 {
		t.Fatalf("got %d results", len(doc.Results))
	}
	wantKeys := []string{"address", "error", "latency_ms", "name", "status"}
	for i, r := range doc.Results {
		if len(r) != len(wantKeys) {
			t.Errorf("result %d has %d keys, want %d: %v", i, len(r), len(wantKeys), r)
		}
		for _, k := range wantKeys {
			if _, ok := r[k]; !ok {
				t.Errorf("result %d missing key %q", i, k)
			}
		}
	}

	first, second := doc.Results[0], doc.Results[1]
	if first["name"] != "API" || first["status"] != "open" || first["error"] != nil {
		t.Errorf("unexpected first result: %v", first)
	}
	if first["latency_ms"] != 1.235 {
		t.Errorf("latency_ms = %v, want 1.235", first["latency_ms"])
	}
	if second["name"] != "" || second["error"] != "connection refused" {
		t.Errorf("unexpected second result: %v", second)
	}
	if doc.Summary["total"] != 2.0 || doc.Summary["open"] != 1.0 || doc.Summary["failed"] != 1.0 {
		t.Errorf("unexpected summary: %v", doc.Summary)
	}
}
