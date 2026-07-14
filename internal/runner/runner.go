// Package runner executes a capability's argv directly (no shell, ADR-0003) and
// classifies the outcome into pass/fail/error (ADR-0002).
//
// The command runs in its own process group so a timeout kills the whole tree,
// not just the direct child. Combined stdout+stderr is captured in memory (never
// persisted, ADR-0005) so the caller can print it on a red and drop it on a green.
package runner

import (
	"os/exec"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/erikolson/ratchet/internal/verdict"
)

// maxCapture bounds in-memory output so a runaway process cannot exhaust memory.
const maxCapture = 1 << 20 // 1 MiB

// Outcome is the classified result of running one capability.
type Outcome struct {
	Status   verdict.Status
	ExitCode int    // process exit code; -1 when signaled or never exited
	Reason   string // populated when Status is error
	Output   []byte // captured combined stdout+stderr (for display on red)
	Duration time.Duration
	TimedOut bool
}

// cappedBuffer accumulates output up to a cap, then drops the rest.
type cappedBuffer struct {
	buf       []byte
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := maxCapture - len(c.buf); room > 0 {
		if len(p) > room {
			c.buf = append(c.buf, p[:room]...)
			c.truncated = true
		} else {
			c.buf = append(c.buf, p...)
		}
	} else {
		c.truncated = true
	}
	return len(p), nil // always report full write; capping is not the process's problem
}

// Run executes argv in dir, classifying the exit against the pass/fail code sets.
// It never returns an error: every failure mode is encoded in the Outcome.
func Run(dir string, argv []string, pass, fail []int, timeout time.Duration) Outcome {
	start := time.Now()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	// Env is inherited unmodified (ADR-0003): exec uses os.Environ when Env==nil.
	var buf cappedBuffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	setProcGroup(cmd) // own process group (platform hook)

	if err := cmd.Start(); err != nil {
		// Spawn failure, including ENOENT (missing binary) and EACCES (126/127
		// class): the harness could not run, so this is error, not fail.
		return Outcome{
			Status:   verdict.StatusError,
			ExitCode: -1,
			Reason:   "could not start command: " + err.Error(),
			Output:   buf.buf,
			Duration: time.Since(start),
		}
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return classify(err, pass, fail, buf.buf, time.Since(start))
	case <-time.After(timeout):
		killGroup(cmd.Process.Pid) // platform hook: kills the whole group
		<-done                     // reap
		return Outcome{
			Status:   verdict.StatusError,
			ExitCode: -1,
			Reason:   "timed out after " + timeout.String(),
			Output:   buf.buf,
			Duration: time.Since(start),
			TimedOut: true,
		}
	}
}

func classify(waitErr error, pass, fail []int, output []byte, dur time.Duration) Outcome {
	o := Outcome{ExitCode: -1, Output: output, Duration: dur}

	if waitErr == nil {
		return classifyCode(o, 0, pass, fail)
	}

	exitErr, ok := waitErr.(*exec.ExitError)
	if !ok {
		o.Status = verdict.StatusError
		o.Reason = "process wait failed: " + waitErr.Error()
		return o
	}
	if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		o.Status = verdict.StatusError
		o.Reason = "killed by signal " + ws.Signal().String()
		return o
	}
	return classifyCode(o, exitErr.ExitCode(), pass, fail)
}

func classifyCode(o Outcome, code int, pass, fail []int) Outcome {
	o.ExitCode = code
	// 126/127 are structural (not executable / not found) and are always error,
	// regardless of the manifest's code sets (ADR-0002).
	if code == 126 || code == 127 {
		o.Status = verdict.StatusError
		o.Reason = "structural exit code " + strconv.Itoa(code) + " (command not executable or not found)"
		return o
	}
	if slices.Contains(pass, code) {
		o.Status = verdict.StatusPass
		return o
	}
	if slices.Contains(fail, code) {
		o.Status = verdict.StatusFail
		return o
	}
	o.Status = verdict.StatusError
	o.Reason = "unexpected exit code " + strconv.Itoa(code) + " (not in pass or fail)"
	return o
}
