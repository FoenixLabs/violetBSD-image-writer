//go:build linux || darwin

package disks

import "os/exec"

func configureCommand(cmd *exec.Cmd) {}
