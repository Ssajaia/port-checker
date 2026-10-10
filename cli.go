package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	version        = "0.1.0"
	defaultTimeout = 5 * time.Second
	maxConcurrent  = 16
)

const (
	exitOK     = 0
	exitFailed = 1
	exitError  = 2
)

const usageText = `Portpeek checks TCP connectivity to host:port endpoints.

Usage:
  portpeek [options] <host:port>
  portpeek [options] check <config-file>

Options:
  --timeout <duration>  Connection timeout such as 500ms, 3s or 1m
                        (default 5s; overrides the timeout in a config file)
  --json                Print results as JSON on stdout
  --version             Print version and exit
  -h, --help            Print this help and exit

Exit codes:
  0  all checks succeeded
  1  one or more checks failed
  2  invalid arguments, invalid configuration or other error

A successful check means a TCP connection was established. It does not
mean the service behind the port is healthy.
`

type options struct {
	timeout    time.Duration
	timeoutSet bool
	json       bool
	help       bool
	version    bool
	args       []string
}

func parseArgs(args []string) (options, error) {
	var o options
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			o.args = append(o.args, args[i+1:]...)
			return o, nil
		case a == "--help" || a == "-h":
			o.help = true
		case a == "--version":
			o.version = true
		case a == "--json":
			o.json = true
		case a == "--timeout" || strings.HasPrefix(a, "--timeout="):
			var value string
			if a == "--timeout" {
				if i+1 >= len(args) {
					return options{}, fmt.Errorf("--timeout requires a value such as 3s")
				}
				i++
				value = args[i]
			} else {
				value = strings.TrimPrefix(a, "--timeout=")
			}
			d, err := parseTimeout(value)
			if err != nil {
				return options{}, err
			}
			o.timeout, o.timeoutSet = d, true
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return options{}, fmt.Errorf("unknown option %q", a)
		default:
			o.args = append(o.args, a)
		}
	}
	return o, nil
}

func pickTimeout(o options, cfg Config) time.Duration {
	switch {
	case o.timeoutSet:
		return o.timeout
	case cfg.Timeout > 0:
		return cfg.Timeout
	default:
		return defaultTimeout
	}
}

func run(args []string, stdout, stderr io.Writer) int {
	o, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "portpeek: %v\nRun 'portpeek --help' for usage.\n", err)
		return exitError
	}
	if o.help {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}
	if o.version {
		fmt.Fprintf(stdout, "portpeek %s\n", version)
		return exitOK
	}

	var targets []Target
	var cfg Config
	switch {
	case len(o.args) == 0:
		fmt.Fprint(stderr, "portpeek: missing argument: expected <host:port> or 'check <config-file>'\n\n"+usageText)
		return exitError
	case o.args[0] == "check":
		if len(o.args) != 2 {
			fmt.Fprint(stderr, "portpeek: 'check' requires exactly one config file path\nRun 'portpeek --help' for usage.\n")
			return exitError
		}
		cfg, err = loadConfig(o.args[1])
		if err != nil {
			fmt.Fprintf(stderr, "portpeek: %v\n", err)
			return exitError
		}
		targets = cfg.Targets
	case len(o.args) == 1:
		if err := validateAddress(o.args[0]); err != nil {
			fmt.Fprintf(stderr, "portpeek: invalid address %q: %v\n", o.args[0], err)
			return exitError
		}
		targets = []Target{{Address: o.args[0]}}
	default:
		fmt.Fprintf(stderr, "portpeek: expected a single <host:port>, got %d arguments\nRun 'portpeek --help' for usage.\n", len(o.args))
		return exitError
	}

	results := runChecks(targets, pickTimeout(o, cfg), maxConcurrent, checkTarget)

	if o.json {
		err = writeJSON(stdout, results)
	} else {
		err = writeHuman(stdout, results)
	}
	if err != nil {
		fmt.Fprintf(stderr, "portpeek: cannot write output: %v\n", err)
		return exitError
	}
	if countOpen(results) != len(results) {
		return exitFailed
	}
	return exitOK
}
