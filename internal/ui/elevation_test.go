package ui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"violetbsd.org/image-writer/internal/disks"
)

func TestAuthorizationWorkerProtocol(t *testing.T) {
	for _, mode := range []string{"success", "cancel", "missing", "failure", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestMockAuthorizationProcess$")
			cmd.Env = append(os.Environ(), "VIOLET_AUTH_TEST="+mode)
			progress := false
			err := runAuthorized(cmd, func(s string, f float64) {
				if s == "Verifying" && f == 0.75 {
					progress = true
				}
			})
			if mode == "success" {
				if err != nil || !progress {
					t.Fatalf("success: progress=%v error=%v", progress, err)
				}
			} else if err == nil {
				t.Fatal("failed/cancelled worker reported success")
			}
			if mode == "failure" && !strings.Contains(err.Error(), "drive changed") {
				t.Fatal(err)
			}
		})
	}
}
func TestMockAuthorizationProcess(t *testing.T) {
	switch os.Getenv("VIOLET_AUTH_TEST") {
	case "":
		return
	case "success":
		fmt.Println(`{"Status":"Verifying","Fraction":0.75}`)
		fmt.Println(`{"Done":true}`)
	case "cancel":
		fmt.Fprintln(os.Stderr, "Authorization cancelled")
		os.Exit(126)
	case "missing":
		fmt.Println(`{"Status":"Verifying","Fraction":0.75}`)
	case "failure":
		fmt.Println(`{"Done":true,"Error":"drive changed"}`)
		os.Exit(1)
	case "invalid":
		fmt.Println("not JSON")
		fmt.Println(`{"Done":true}`)
	}
	os.Exit(0)
}
func TestWriteRequest(t *testing.T) {
	request := writeRequest{filepath.Join(t.TempDir(), "image 'with quotes' $(no-shell).img"), disks.Drive{Path: "/dev/example", Size: 4096, Identity: "device-id"}}
	b, _ := json.Marshal(request)
	decoded, err := decodeRequest(base64.StdEncoding.EncodeToString(b))
	if err != nil || decoded != request {
		t.Fatalf("round trip: %v", err)
	}
	for _, bad := range []string{"!invalid!", strings.Repeat("x", 65537), base64.StdEncoding.EncodeToString([]byte(`{"Image":"relative","Drive":{}}`)), base64.StdEncoding.EncodeToString(append(b, []byte(` {}`)...)), base64.StdEncoding.EncodeToString([]byte(`{"Unknown":true}`))} {
		if _, err := decodeRequest(bad); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}
