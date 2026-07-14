//go:build unix

package runner

import (
	"os/exec"
	"syscall"
)

// setProcGroup puts the child in its own process group so its descendants can be
// killed as a unit.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup SIGKILLs the entire process group led by pid. The negative pid
// targets the group, so backgrounded grandchildren die with the parent.
func killGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
