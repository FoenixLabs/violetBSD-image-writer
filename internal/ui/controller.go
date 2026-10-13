package ui

import (
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"violetbsd.org/image-writer/internal/disks"
	"violetbsd.org/image-writer/internal/writer"
)

const Title = "violetBSD Image Writer"

type Controller struct {
	Drives       []disks.Drive // Accessed only on the UI thread.
	mu           sync.Mutex
	busy         bool
	status       string
	fraction     float64
	progressHook func(string, float64)
}

func Run() error {
	return runNative(&Controller{status: "Select an image and an unmounted external drive."})
}
func (c *Controller) Snapshot() (bool, string, float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.busy, c.status, c.fraction
}
func (c *Controller) report(status string, fraction float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = status
	c.fraction = fraction
	if c.progressHook != nil {
		c.progressHook(status, fraction)
	}
}
func (c *Controller) Refresh() error {
	c.Drives = nil
	all, err := disks.Discover()
	if err != nil {
		return err
	}
	c.Drives = all
	if len(all) == 0 {
		c.report("No eligible drives. Unmount external media, then Refresh.", 0)
	} else {
		c.report("Select an image and destination.", 0)
	}
	return nil
}
func (c *Controller) Validate(path string, index int) (disks.Drive, error) {
	if busy, _, _ := c.Snapshot(); busy {
		return disks.Drive{}, fmt.Errorf("a write is already running")
	}
	if index < 0 || index >= len(c.Drives) {
		return disks.Drive{}, fmt.Errorf("select a destination drive")
	}
	f, err := os.Open(path)
	if err != nil {
		return disks.Drive{}, err
	}
	defer f.Close()
	if _, err = disks.CheckImageFile(f); err != nil {
		return disks.Drive{}, err
	}
	d := c.Drives[index]
	if err = disks.CheckSource(path, d); err != nil {
		return disks.Drive{}, err
	}
	if err = disks.Recheck(d); err != nil {
		return disks.Drive{}, err
	}
	return d, nil
}

// Start is called only after a native confirmation dialog names the selected drive.
func (c *Controller) Start(path string, d disks.Drive) {
	c.mu.Lock()
	if c.busy {
		c.mu.Unlock()
		return
	}
	c.busy = true
	c.status = "Checking image…"
	c.fraction = 0
	c.mu.Unlock()
	go func() {
		err := c.authorizedWrite(path, d)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.busy = false
		if err != nil {
			c.status = "Failed: " + err.Error() + ". If writing began, rewrite the destination before use."
		} else {
			c.status = "Verified. Unique entropy is ready for first boot. Safely remove the drive."
			c.fraction = 1
		}
	}()
}
func (c *Controller) write(path string, d disks.Drive) error {
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { source.Close() }()
	before, err := disks.CheckImageFile(source)
	if err != nil {
		return err
	}
	size := before.Size()
	var offset int64
	compressed := strings.HasSuffix(strings.ToLower(path), ".gz")
	var expectedSource [32]byte
	if compressed {
		c.report("Checking compressed image and entropy slot without temporary extraction…", 0)
		size, offset, expectedSource, err = writer.InspectGzip(source, d.Size)
	} else {
		if size > d.Size || size%512 != 0 {
			return fmt.Errorf("image size does not fit the destination")
		}
		offset, err = writer.SeedOffset(source, size)
	}
	if err != nil {
		return err
	}
	seed, err := writer.Seed()
	if err != nil {
		return err
	}
	defer clear(seed)
	if _, err = source.Seek(0, 0); err != nil {
		return err
	}
	var input io.Reader = source
	sourceHash := sha256.New()
	if compressed {
		gz, err := gzip.NewReader(source)
		if err != nil {
			return err
		}
		defer gz.Close()
		input = io.TeeReader(gz, sourceHash)
	}
	target, err := disks.Open(d)
	if err != nil {
		return err
	}
	defer target.Close()
	digest, err := writer.Copy(target, input, size, offset, seed, func(n int64) {
		c.report("Writing image and unique entropy seed…", .5*float64(n)/float64(size))
	})
	if err != nil {
		return err
	}
	if compressed {
		// Force gzip trailer/checksum validation and reject a changed stream length.
		var extra [1]byte
		if n, e := input.Read(extra[:]); n != 0 || e != io.EOF {
			return fmt.Errorf("compressed image changed or failed its final checksum: %v", e)
		}
		var actual [32]byte
		copy(actual[:], sourceHash.Sum(nil))
		if actual != expectedSource {
			return fmt.Errorf("compressed source changed after validation; rewrite destination")
		}
	}
	after, err := source.Stat()
	if err != nil {
		return err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return fmt.Errorf("source changed during write")
	}
	c.report("Flushing destination…", .5)
	if err = disks.Flush(target); err != nil {
		return err
	}
	if _, err = target.Seek(0, 0); err != nil {
		return err
	}
	if err = writer.Verify(target, size, digest, func(n int64) {
		c.report("Verifying the entire written image…", .5+.5*float64(n)/float64(size))
	}); err != nil {
		return err
	}
	return target.Close()
}
