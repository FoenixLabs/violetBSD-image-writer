package ui

import (
	"fmt"
	"os"
	"os/exec"
)

func authorizationCommand(executable, request string) (*exec.Cmd, error) {
	if _, err := os.Stat("/usr/bin/pkexec"); err != nil {
		return nil, fmt.Errorf("install polkit and enable your desktop's polkit authentication agent to show the administrator password prompt: %w", err)
	}
	// No shell and no forwarded display or user-controlled environment. The GUI
	// stays unprivileged; only the headless disk writer is authorized by polkit.
	return exec.Command("/usr/bin/pkexec", "--disable-internal-agent", executable, "--elevated-write", request), nil
}
