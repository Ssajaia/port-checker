package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"
)

const (
	statusOpen    = "open"
	statusRefused = "refused"
	statusTimeout = "timeout"
	statusDNS     = "dns_error"
	statusError   = "error"
)

const windowsConnRefused = syscall.Errno(10061)

type Target struct {
	Name    string
	Address string
}

type Result struct {
	Target  Target
	Status  string
	Latency time.Duration
	Err     string
}

type checkFunc func(Target, time.Duration) Result

func checkTarget(t Target, timeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var dialer net.Dialer
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", t.Address)
	elapsed := time.Since(start)
	if err != nil {
		status := classifyError(err)
		return Result{Target: t, Status: status, Latency: elapsed, Err: describeError(err, status, timeout)}
	}
	if err := conn.Close(); err != nil {
		return Result{Target: t, Status: statusError, Latency: elapsed, Err: err.Error()}
	}
	return Result{Target: t, Status: statusOpen, Latency: elapsed}
}

func classifyError(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return statusDNS
	}
	var errno syscall.Errno
	if errors.Is(err, syscall.ECONNREFUSED) || (errors.As(err, &errno) && errno == windowsConnRefused) {
		return statusRefused
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return statusTimeout
	}
	return statusError
}

func describeError(err error, status string, timeout time.Duration) string {
	switch status {
	case statusRefused:
		return "connection refused"
	case statusTimeout:
		return fmt.Sprintf("no response within %s", timeout)
	default:
		return err.Error()
	}
}

func runChecks(targets []Target, timeout time.Duration, limit int, check checkFunc) []Result {
	results := make([]Result, len(targets))
	if limit < 1 {
		limit = 1
	}
	slots := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, t := range targets {
		slots <- struct{}{}
		wg.Add(1)
		go func(i int, t Target) {
			defer wg.Done()
			defer func() { <-slots }()
			results[i] = check(t, timeout)
		}(i, t)
	}
	wg.Wait()
	return results
}
