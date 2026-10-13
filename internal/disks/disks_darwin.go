package disks

import (
	"fmt"
	"golang.org/x/sys/unix"
	"howett.net/plist"
	"os"
	"unsafe"
)

func Discover() ([]Drive, error) {
	b, err := command("/usr/sbin/diskutil", "list", "-plist", "external", "physical")
	if err != nil {
		return nil, err
	}
	var data struct{ AllDisksAndPartitions []macDisk }
	if _, err = plist.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	var result []Drive
	for _, d := range data.AllDisksAndPartitions {
		if !macID.MatchString(d.DeviceIdentifier) {
			continue
		}
		b, err = command("/usr/sbin/diskutil", "info", "-plist", d.DeviceIdentifier)
		if err != nil {
			return nil, err
		}
		var info macInfo
		if _, err = plist.Unmarshal(b, &info); err != nil {
			return nil, err
		}
		if macEligible(d, info) {
			result = append(result, Drive{"/dev/r" + d.DeviceIdentifier, info.MediaName, "", info.TotalSize, info.DeviceTreePath})
		}
	}
	return result, nil
}
func Open(d Drive) (*os.File, error) {
	if err := Recheck(d); err != nil {
		return nil, err
	}
	fd, err := unix.Open(d.Path, unix.O_RDWR|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), d.Path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		f.Close()
		return nil, fmt.Errorf("destination must be a raw character device")
	}
	// Verify actual device geometry after opening it, not just diskutil's snapshot.
	sector, err := unix.IoctlGetInt(fd, 0x40046418) // DKIOCGETBLOCKSIZE
	if err != nil || sector != 512 {
		f.Close()
		return nil, fmt.Errorf("unsupported destination sector size: %v", err)
	}
	var blocks uint64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), 0x40086419, uintptr(unsafe.Pointer(&blocks))) // DKIOCGETBLOCKCOUNT
	if errno != 0 || blocks != uint64(d.Size/512) {
		f.Close()
		return nil, fmt.Errorf("destination capacity changed: %v", errno)
	}
	writable, err := unix.IoctlGetInt(fd, 0x4004641d) // DKIOCISWRITABLE
	if err != nil || writable == 0 {
		f.Close()
		return nil, fmt.Errorf("destination is not writable: %v", err)
	}
	// F_NOCACHE keeps verification reads out of the unified buffer cache.
	if _, err = unix.FcntlInt(f.Fd(), unix.F_NOCACHE, 1); err != nil {
		f.Close()
		return nil, err
	}
	if err = Recheck(d); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func Flush(f *os.File) error {
	if err := f.Sync(); err != nil {
		return err
	}
	// Apple's raw-disk flush ioctl, rather than a regular-file-only flush path.
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), 0x20006416, 0) // DKIOCSYNCHRONIZECACHE
	if errno != 0 {
		return errno
	}
	return nil
}

func CheckSource(path string, d Drive) error { return nil }
func Privileged() bool                       { return os.Geteuid() == 0 }
