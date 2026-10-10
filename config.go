package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Targets []Target
	Timeout time.Duration
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("cannot read config file: %w", err)
	}
	return parseConfig(path, data)
}

func parseTimeout(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: use a duration such as 500ms, 3s or 1m", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid timeout %q: must be greater than zero", s)
	}
	return d, nil
}

func lineError(name string, line int, format string, args ...any) error {
	return fmt.Errorf("%s:%d: %s", name, line, fmt.Sprintf(format, args...))
}

func parseConfig(name string, data []byte) (Config, error) {
	var cfg Config
	var targetLines []int
	var fields map[string]bool
	seenTargets, seenTimeout, inTargets := false, false, false
	current := -1

	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	for i, raw := range strings.Split(text, "\n") {
		n := i + 1
		line := strings.TrimRight(raw, " \t\r")
		body := strings.TrimLeft(line, " ")
		if body == "" || strings.HasPrefix(body, "#") {
			continue
		}
		if strings.HasPrefix(body, "\t") {
			return Config{}, lineError(name, n, "tabs are not allowed for indentation; use spaces")
		}
		indent := len(line) - len(body)
		isItem := body == "-" || strings.HasPrefix(body, "- ")

		if indent == 0 && !isItem {
			key, value, ok := splitKeyValue(body)
			if !ok {
				return Config{}, lineError(name, n, "expected 'key: value', got %q", body)
			}
			switch key {
			case "targets":
				if seenTargets {
					return Config{}, lineError(name, n, "duplicate key 'targets'")
				}
				if value != "" {
					return Config{}, lineError(name, n, "'targets' must be followed by a list of targets on the next lines")
				}
				seenTargets, inTargets = true, true
			case "timeout":
				if seenTimeout {
					return Config{}, lineError(name, n, "duplicate key 'timeout'")
				}
				d, err := parseTimeout(unquote(value))
				if err != nil {
					return Config{}, lineError(name, n, "%v", err)
				}
				cfg.Timeout, seenTimeout, inTargets = d, true, false
			default:
				return Config{}, lineError(name, n, "unknown key %q (supported keys: targets, timeout)", key)
			}
			continue
		}

		if !inTargets {
			return Config{}, lineError(name, n, "unexpected indented line; indentation is only valid under 'targets'")
		}
		if isItem {
			cfg.Targets = append(cfg.Targets, Target{})
			targetLines = append(targetLines, n)
			current = len(cfg.Targets) - 1
			fields = map[string]bool{}
			body = strings.TrimLeft(strings.TrimPrefix(body, "-"), " ")
			if body == "" {
				continue
			}
		} else if current < 0 {
			return Config{}, lineError(name, n, "expected a target starting with '- '")
		}

		key, value, ok := splitKeyValue(body)
		if !ok {
			return Config{}, lineError(name, n, "expected 'key: value', got %q", body)
		}
		if fields[key] {
			return Config{}, lineError(name, n, "duplicate field %q in target", key)
		}
		fields[key] = true
		value = unquote(value)
		switch key {
		case "name":
			cfg.Targets[current].Name = value
		case "address":
			cfg.Targets[current].Address = value
		default:
			return Config{}, lineError(name, n, "unknown field %q (supported fields: name, address)", key)
		}
	}

	if err := validateConfig(name, cfg, targetLines); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateConfig(name string, cfg Config, targetLines []int) error {
	if len(cfg.Targets) == 0 {
		return fmt.Errorf("%s: 'targets' is required and must contain at least one target", name)
	}
	names := map[string]int{}
	addresses := map[string]int{}
	for i, t := range cfg.Targets {
		line := targetLines[i]
		if t.Address == "" {
			return lineError(name, line, "target #%d is missing required field 'address'", i+1)
		}
		if err := validateAddress(t.Address); err != nil {
			return lineError(name, line, "target #%d has invalid address %q: %v", i+1, t.Address, err)
		}
		if t.Name != "" {
			if first, dup := names[t.Name]; dup {
				return lineError(name, line, "target #%d reuses name %q already used by target #%d", i+1, t.Name, first)
			}
			names[t.Name] = i + 1
		}
		if first, dup := addresses[t.Address]; dup {
			return lineError(name, line, "target #%d repeats address %q already used by target #%d", i+1, t.Address, first)
		}
		addresses[t.Address] = i + 1
	}
	return nil
}

func splitKeyValue(s string) (string, string, bool) {
	var key, value string
	if i := strings.Index(s, ": "); i >= 0 {
		key, value = s[:i], s[i+2:]
	} else if strings.HasSuffix(s, ":") {
		key = strings.TrimSuffix(s, ":")
	} else {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(value), true
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
