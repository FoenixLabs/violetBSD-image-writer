package ui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacAuthorizationScriptCompiles(t *testing.T) {
	// Compile only: never execute a script or request real administrator access.
	cmd, err := authorizationCommand("/Applications/User's \"Writer\" $(literal)/writer", "eyJ0ZXN0Ijp0cnVlfQ==")
	if err != nil {
		t.Fatal(err)
	}
	compile := exec.Command("/usr/bin/osacompile", "-o", filepath.Join(t.TempDir(), "authorize.scpt"))
	compile.Stdin = strings.NewReader(cmd.Args[2])
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("invalid authorization script: %v: %s", err, out)
	}
}
