package ui

import (
	"os/exec"
	"strings"
)

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func appleQuote(s string) string {
	return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r").Replace(s) + "\""
}
func authorizationCommand(executable, request string) (*exec.Cmd, error) {
	command := shellQuote(executable) + " --elevated-write " + shellQuote(request)
	script := "with timeout of 86400 seconds\nreturn do shell script " + appleQuote(command) + " with administrator privileges without altering line endings\nend timeout"
	return exec.Command("/usr/bin/osascript", "-e", script), nil
}
