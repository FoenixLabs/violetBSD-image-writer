package ui

import (
	"fmt"
	"os/exec"
)

func authorizationCommand(executable, request string) (*exec.Cmd, error) {
	return nil, fmt.Errorf("administrator access was not granted; launch the release executable and approve its Windows UAC prompt")
}
