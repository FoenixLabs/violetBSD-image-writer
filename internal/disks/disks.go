// Package disks implements deliberately conservative whole-drive access.
package disks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type Drive struct {
	Path     string
	Name     string
	Serial   string
	Size     int64
	Identity string
}

func (d Drive) Label() string {
	return fmt.Sprintf("%s — %.1f GB — %s", d.Name, float64(d.Size)/1e9, d.Path)
}

func command(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	configureCommand(cmd)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, b)
	}
	return b, nil
}
func Recheck(d Drive) error {
	all, err := Discover()
	if err != nil {
		return err
	}
	for _, current := range all {
		if current == d {
			return nil
		}
	}
	return fmt.Errorf("drive changed or became ineligible; refresh the destination list")
}
func CheckImageFile(f *os.File) (os.FileInfo, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("image must be a regular file")
	}
	return info, nil
}
