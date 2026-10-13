package ui

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"violetbsd.org/image-writer/internal/disks"
)

type writeRequest struct {
	Image string
	Drive disks.Drive
}
type writeEvent struct {
	Status   string
	Fraction float64
	Done     bool
	Error    string
}

// ElevatedWrite is an internal entry point for the OS-authorized worker. It
// rechecks the selected drive and source; it never trusts discovery from the UI.
func ElevatedWrite(encoded string) error {
	if !disks.Privileged() {
		return fmt.Errorf("write helper requires OS administrator authorization")
	}
	request, err := decodeRequest(encoded)
	if err != nil {
		return err
	}
	c := &Controller{}
	encoder := json.NewEncoder(os.Stdout)
	var last time.Time
	c.progressHook = func(status string, fraction float64) {
		if time.Since(last) >= 250*time.Millisecond {
			_ = encoder.Encode(writeEvent{Status: status, Fraction: fraction})
			last = time.Now()
		}
	}
	if err = disks.CheckSource(request.Image, request.Drive); err == nil {
		err = disks.Recheck(request.Drive)
	}
	if err == nil {
		err = c.write(request.Image, request.Drive)
	}
	event := writeEvent{Done: true, Fraction: 1}
	if err != nil {
		event.Error = err.Error()
		event.Fraction = 0
	}
	if encodeErr := encoder.Encode(event); encodeErr != nil {
		return encodeErr
	}
	return err
}
func decodeRequest(encoded string) (writeRequest, error) {
	var r writeRequest
	if len(encoded) > 65536 {
		return r, fmt.Errorf("write request too large")
	}
	b, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil {
		return r, e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&r); e != nil {
		return r, e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return r, fmt.Errorf("trailing write request data")
	}
	if !filepath.IsAbs(r.Image) || r.Drive.Path == "" || r.Drive.Size <= 0 || r.Drive.Identity == "" {
		return r, fmt.Errorf("invalid write request")
	}
	return r, nil
}
func (c *Controller) authorizedWrite(path string, d disks.Drive) error {
	if disks.Privileged() {
		return c.write(path, d)
	}
	image, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	request, err := json.Marshal(writeRequest{image, d})
	if err != nil {
		return err
	}
	cmd, err := authorizationCommand(exe, base64.StdEncoding.EncodeToString(request))
	if err != nil {
		return err
	}
	c.report("Approve the operating system's administrator password prompt to write the selected drive…", 0)
	return runAuthorized(cmd, c.report)
}
func runAuthorized(cmd *exec.Cmd, report func(string, float64)) error {
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("could not request administrator access: %w", err)
	}
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var done bool
	var workerErr error
	for scanner.Scan() {
		var event writeEvent
		if err = json.Unmarshal(scanner.Bytes(), &event); err != nil {
			workerErr = fmt.Errorf("invalid privileged worker response")
			continue
		}
		if event.Done {
			done = true
			if event.Error != "" {
				workerErr = fmt.Errorf("%s", event.Error)
			}
		} else if event.Status != "" {
			report(event.Status, event.Fraction)
		}
	}
	scanErr := scanner.Err()
	// Drain on oversized/invalid output so the worker is never blocked mid-write.
	if scanErr != nil {
		_, _ = io.Copy(io.Discard, pipe)
	}
	exitErr := cmd.Wait()
	if workerErr != nil {
		return workerErr
	}
	if scanErr != nil {
		return scanErr
	}
	if exitErr != nil {
		return fmt.Errorf("administrator authorization or write helper failed: %w. %s", exitErr, strings.TrimSpace(diagnostics.String()))
	}
	if !done {
		return fmt.Errorf("administrator helper exited without confirming a verified write")
	}
	return nil
}
