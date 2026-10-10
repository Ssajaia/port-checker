package main

import (
	"context"
	"errors"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timed out" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

func startListener(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return l
}

func closedAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return addr
}

func TestCheckTargetOpen(t *testing.T) {
	l := startListener(t)
	res := checkTarget(Target{Name: "local", Address: l.Addr().String()}, 2*time.Second)
	if res.Status != statusOpen {
		t.Fatalf("status = %q (%s), want %q", res.Status, res.Err, statusOpen)
	}
	if res.Err != "" {
		t.Errorf("unexpected error text %q", res.Err)
	}
	if res.Latency < 0 || res.Latency > 2*time.Second {
		t.Errorf("latency out of range: %v", res.Latency)
	}
	if res.Target.Name != "local" {
		t.Errorf("target name not preserved: %q", res.Target.Name)
	}
}

func TestCheckTargetRefused(t *testing.T) {
	res := checkTarget(Target{Address: closedAddress(t)}, 5*time.Second)
	if res.Status != statusRefused {
		t.Fatalf("status = %q (%s), want %q", res.Status, res.Err, statusRefused)
	}
	if res.Err != "connection refused" {
		t.Errorf("error = %q", res.Err)
	}
}

func TestCheckTargetExpiredDeadlineIsTimeout(t *testing.T) {
	l := startListener(t)
	res := checkTarget(Target{Address: l.Addr().String()}, -time.Second)
	if res.Status != statusTimeout {
		t.Fatalf("status = %q (%s), want %q", res.Status, res.Err, statusTimeout)
	}
}

func TestCheckTargetDNSFailure(t *testing.T) {
	res := checkTarget(Target{Address: "portpeek-test.invalid:80"}, 5*time.Second)
	if res.Status != statusDNS {
		t.Fatalf("status = %q (%s), want %q", res.Status, res.Err, statusDNS)
	}
	if res.Err == "" {
		t.Error("expected error text for DNS failure")
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"dns not found", &net.DNSError{Err: "no such host", Name: "x.invalid", IsNotFound: true}, statusDNS},
		{"dns timeout", &net.DNSError{Err: "i/o timeout", Name: "x.invalid", IsTimeout: true}, statusDNS},
		{"refused unix style", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, statusRefused},
		{"refused windows style", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.Errno(10061))}, statusRefused},
		{"net timeout", &net.OpError{Op: "dial", Net: "tcp", Err: timeoutErr{}}, statusTimeout},
		{"context deadline", context.DeadlineExceeded, statusTimeout},
		{"wrapped context deadline", &net.OpError{Op: "dial", Err: context.DeadlineExceeded}, statusTimeout},
		{"other", errors.New("boom"), statusError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyError(c.err); got != c.want {
				t.Errorf("classifyError = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRunChecksPreservesOrder(t *testing.T) {
	targets := make([]Target, 8)
	for i := range targets {
		targets[i] = Target{Address: "host:" + string(rune('1'+i))}
	}
	slowFirst := func(tg Target, _ time.Duration) Result {
		idx := int(tg.Address[len(tg.Address)-1] - '1')
		time.Sleep(time.Duration(len(targets)-idx) * 15 * time.Millisecond)
		return Result{Target: tg, Status: statusOpen}
	}
	results := runChecks(targets, time.Second, len(targets), slowFirst)
	if len(results) != len(targets) {
		t.Fatalf("got %d results, want %d", len(results), len(targets))
	}
	for i, r := range results {
		if r.Target != targets[i] {
			t.Errorf("results[%d].Target = %+v, want %+v", i, r.Target, targets[i])
		}
	}
}

func TestRunChecksRespectsLimit(t *testing.T) {
	const limit = 3
	targets := make([]Target, 12)
	var current, peak atomic.Int32
	check := func(tg Target, _ time.Duration) Result {
		n := current.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		current.Add(-1)
		return Result{Target: tg, Status: statusOpen}
	}
	runChecks(targets, time.Second, limit, check)
	if p := peak.Load(); p > limit || p < 1 {
		t.Errorf("peak concurrency = %d, want between 1 and %d", p, limit)
	}
	if current.Load() != 0 {
		t.Errorf("checks still running after runChecks returned")
	}
}

func TestRunChecksLimitBelowOneRunsSequentially(t *testing.T) {
	var current, peak atomic.Int32
	check := func(tg Target, _ time.Duration) Result {
		n := current.Add(1)
		if n > peak.Load() {
			peak.Store(n)
		}
		time.Sleep(5 * time.Millisecond)
		current.Add(-1)
		return Result{Target: tg, Status: statusOpen}
	}
	runChecks(make([]Target, 4), time.Second, 0, check)
	if peak.Load() != 1 {
		t.Errorf("peak concurrency = %d, want 1", peak.Load())
	}
}

func TestRunChecksEmpty(t *testing.T) {
	if got := runChecks(nil, time.Second, 4, checkTarget); len(got) != 0 {
		t.Errorf("got %d results for no targets", len(got))
	}
}

func TestRunChecksMixedRealTargets(t *testing.T) {
	open1 := startListener(t).Addr().String()
	open2 := startListener(t).Addr().String()
	closed := closedAddress(t)
	targets := []Target{{Address: open1}, {Address: closed}, {Address: open2}}
	results := runChecks(targets, 5*time.Second, 2, checkTarget)
	want := []string{statusOpen, statusRefused, statusOpen}
	for i, r := range results {
		if r.Target.Address != targets[i].Address {
			t.Errorf("results[%d] address = %q, want %q", i, r.Target.Address, targets[i].Address)
		}
		if r.Status != want[i] {
			t.Errorf("results[%d] status = %q (%s), want %q", i, r.Status, r.Err, want[i])
		}
	}
}
