//go:build !darwin && !linux

package scriptnode

import "os/exec"

func configureProcessGroup(command *exec.Cmd) {
	command.Cancel = func() error { return terminateProcessGroup(command) }
}

func terminateProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}
