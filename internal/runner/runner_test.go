package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erikolson/ratchet/internal/verdict"
)

func requireSh(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
}

func TestRun_Classification(t *testing.T) {
	requireSh(t)
	cases := []struct {
		name     string
		argv     []string
		pass     []int
		fail     []int
		want     verdict.Status
		wantExit int
	}{
		{"zero is pass", []string{"sh", "-c", "exit 0"}, []int{0}, []int{1}, verdict.StatusPass, 0},
		{"one is fail", []string{"sh", "-c", "exit 1"}, []int{0}, []int{1}, verdict.StatusFail, 1},
		{"five is error (not in sets)", []string{"sh", "-c", "exit 5"}, []int{0}, []int{1}, verdict.StatusError, 5},
		{"custom pass set", []string{"sh", "-c", "exit 2"}, []int{0, 2}, []int{1}, verdict.StatusPass, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := Run(t.TempDir(), tc.argv, tc.pass, tc.fail, 10*time.Second)
			if o.Status != tc.want {
				t.Fatalf("status = %q, want %q (reason: %s)", o.Status, tc.want, o.Reason)
			}
			if o.ExitCode != tc.wantExit {
				t.Fatalf("exit = %d, want %d", o.ExitCode, tc.wantExit)
			}
		})
	}
}

func TestRun_MissingBinaryIsError(t *testing.T) {
	o := Run(t.TempDir(), []string{"ratchet-no-such-binary-zzz"}, []int{0}, []int{1}, 10*time.Second)
	if o.Status != verdict.StatusError {
		t.Fatalf("status = %q, want error for missing binary", o.Status)
	}
	if o.Reason == "" {
		t.Fatal("error outcome must carry a reason")
	}
}

func TestRun_SignalDeathIsError(t *testing.T) {
	requireSh(t)
	o := Run(t.TempDir(), []string{"sh", "-c", "kill -9 $$"}, []int{0}, []int{1}, 10*time.Second)
	if o.Status != verdict.StatusError {
		t.Fatalf("status = %q, want error for signal death", o.Status)
	}
}

func TestRun_TimeoutIsErrorAndPrompt(t *testing.T) {
	requireSh(t)
	start := time.Now()
	o := Run(t.TempDir(), []string{"sh", "-c", "sleep 10"}, []int{0}, []int{1}, 200*time.Millisecond)
	elapsed := time.Since(start)
	if o.Status != verdict.StatusError {
		t.Fatalf("status = %q, want error for timeout", o.Status)
	}
	if !o.TimedOut {
		t.Fatal("TimedOut not set")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("timeout took %v, want prompt return", elapsed)
	}
}

func TestRun_CapturesCombinedOutput(t *testing.T) {
	requireSh(t)
	o := Run(t.TempDir(), []string{"sh", "-c", "echo to-stdout; echo to-stderr 1>&2; exit 1"}, []int{0}, []int{1}, 10*time.Second)
	out := string(o.Output)
	if !strings.Contains(out, "to-stdout") || !strings.Contains(out, "to-stderr") {
		t.Fatalf("captured output missing streams: %q", out)
	}
}

func TestRun_TimeoutKillsProcessGroup(t *testing.T) {
	requireSh(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "grandchild-ran")
	// Parent backgrounds a subshell (same process group) and waits. A timeout
	// that killed only the parent would leave the subshell to touch the marker;
	// killing the whole group must prevent it.
	argv := []string{"sh", "-c", `(sleep 3; touch "$1") & wait`, "sh", marker}
	o := Run(dir, argv, []int{0}, []int{1}, 200*time.Millisecond)
	if !o.TimedOut {
		t.Fatal("expected timeout")
	}
	time.Sleep(1500 * time.Millisecond) // long enough for the subshell to have fired
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("grandchild survived timeout: process group was not killed")
	}
}
