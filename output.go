package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

type jsonResult struct {
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Status    string  `json:"status"`
	LatencyMS float64 `json:"latency_ms"`
	Error     *string `json:"error"`
}

type jsonSummary struct {
	Total  int `json:"total"`
	Open   int `json:"open"`
	Failed int `json:"failed"`
}

type jsonOutput struct {
	Results []jsonResult `json:"results"`
	Summary jsonSummary  `json:"summary"`
}

func countOpen(results []Result) int {
	open := 0
	for _, r := range results {
		if r.Status == statusOpen {
			open++
		}
	}
	return open
}

func formatLatency(d time.Duration) string {
	ms := float64(d) / float64(time.Millisecond)
	if ms >= 1000 {
		return fmt.Sprintf("%.2fs", ms/1000)
	}
	return fmt.Sprintf("%.1fms", ms)
}

func writeHuman(w io.Writer, results []Result) error {
	if len(results) == 0 {
		return nil
	}
	named := false
	for _, r := range results {
		if r.Target.Name != "" {
			named = true
		}
	}

	rows := make([][]string, len(results))
	for i, r := range results {
		row := []string{r.Status}
		if named {
			name := r.Target.Name
			if name == "" {
				name = "-"
			}
			row = append(row, name)
		}
		rows[i] = append(row, r.Target.Address, formatLatency(r.Latency), r.Err)
	}

	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for j, cell := range row {
			if len(cell) > widths[j] {
				widths[j] = len(cell)
			}
		}
	}

	var b strings.Builder
	last := len(widths) - 1
	for _, row := range rows {
		var line strings.Builder
		for j, cell := range row {
			line.WriteString(cell)
			if j < last {
				line.WriteString(strings.Repeat(" ", widths[j]-len(cell)+2))
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	if len(results) > 1 {
		open := countOpen(results)
		fmt.Fprintf(&b, "%d checked: %d open, %d failed\n", len(results), open, len(results)-open)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func toJSONResult(r Result) jsonResult {
	ms := float64(r.Latency) / float64(time.Millisecond)
	jr := jsonResult{
		Name:      r.Target.Name,
		Address:   r.Target.Address,
		Status:    r.Status,
		LatencyMS: math.Round(ms*1000) / 1000,
	}
	if r.Err != "" {
		msg := r.Err
		jr.Error = &msg
	}
	return jr
}

func writeJSON(w io.Writer, results []Result) error {
	out := jsonOutput{Results: make([]jsonResult, len(results))}
	for i, r := range results {
		out.Results[i] = toJSONResult(r)
	}
	open := countOpen(results)
	out.Summary = jsonSummary{Total: len(results), Open: open, Failed: len(results) - open}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
