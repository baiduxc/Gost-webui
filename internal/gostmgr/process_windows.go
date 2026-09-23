//go:build windows

package gostmgr

import (
	"os"
	"os/exec"
)

func configureProcess(cmd *exec.Cmd) {}

func terminateProcess(process *os.Process) error {
	return process.Kill()
}
