package disks

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"unsafe"
)

func Discover() ([]Drive, error) {
	b, err := command("/usr/bin/lsblk", "--json", "--bytes", "-o", "PATH,NAME,TYPE,SIZE,MODEL,SERIAL,TRAN,RM,RO,MOUNTPOINTS,MAJ:MIN,LOG-SEC")
	if err != nil {
		return nil, err
	}
	return parseLinux(b)
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
	fail := func(e error) (*os.File, error) { f.Close(); return nil, e }
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return fail(err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFBLK || fmt.Sprintf("%d:%d", unix.Major(st.Rdev), unix.Minor(st.Rdev)) != d.Identity {
		return fail(fmt.Errorf("destination identity changed"))
	}
	var size uint64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return fail(errno)
	}
	sector, err := unix.IoctlGetInt(fd, unix.BLKSSZGET)
	if err != nil {
		return fail(err)
	}
	ro, err := unix.IoctlGetInt(fd, unix.BLKROGET)
	if err != nil {
		return fail(err)
	}
	if size != uint64(d.Size) || sector != 512 || ro != 0 {
		return fail(fmt.Errorf("destination geometry or read-only state changed"))
	}
	if err = Recheck(d); err != nil {
		return fail(err)
	}
	return f, nil
}
func Flush(f *os.File) error {
	if err := f.Sync(); err != nil {
		return err
	}
	return unix.IoctlSetInt(int(f.Fd()), unix.BLKFLSBUF, 0)
}
func CheckSource(path string, d Drive) error { return nil } // Mounted source disks cannot be eligible.
func Privileged() bool                       { return os.Geteuid() == 0 }
