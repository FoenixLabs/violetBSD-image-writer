package disks

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"
)

func ps(script string) ([]byte, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	return command(filepath.Join(dir, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[System.Text.UTF8Encoding]::new(); "+script)
}
func Discover() ([]Drive, error) {
	b, err := ps("ConvertTo-Json -InputObject @(Get-Disk | Select-Object Number,FriendlyName,SerialNumber,UniqueId,Size,IsBoot,IsSystem,IsReadOnly,IsOffline,BusType,LogicalSectorSize)")
	if err != nil {
		return nil, err
	}
	return parseWindows(b)
}
func number(d Drive) (int, error) {
	if !strings.HasPrefix(d.Path, `\\.\PhysicalDrive`) {
		return 0, fmt.Errorf("invalid physical disk path")
	}
	return strconv.Atoi(strings.TrimPrefix(d.Path, `\\.\PhysicalDrive`))
}
func Open(d Drive) (*os.File, error) {
	if err := Recheck(d); err != nil {
		return nil, err
	}
	n, err := number(d)
	if err != nil {
		return nil, err
	}
	// Refuse unsupported removable media instead of writing through online volumes.
	_, err = ps(fmt.Sprintf("Set-Disk -Number %d -IsOffline $true; if (!(Get-Disk -Number %d).IsOffline) { throw 'Destination remains online' }", n, n))
	if err != nil {
		return nil, err
	}
	if err = Recheck(d); err != nil {
		return nil, err
	}
	path, err := windows.UTF16PtrFromString(d.Path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_WRITE_THROUGH, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), d.Path)
	var length int64
	var returned uint32
	err = windows.DeviceIoControl(h, 0x7405c, nil, 0, (*byte)(unsafe.Pointer(&length)), 8, &returned, nil) // IOCTL_DISK_GET_LENGTH_INFO
	if err != nil || returned != 8 || length != d.Size {
		f.Close()
		return nil, fmt.Errorf("destination capacity verification failed: %v", err)
	}
	var geometry [24]byte
	err = windows.DeviceIoControl(h, 0x70000, nil, 0, &geometry[0], uint32(len(geometry)), &returned, nil) // IOCTL_DISK_GET_DRIVE_GEOMETRY
	if err != nil || returned < 24 || binary.LittleEndian.Uint32(geometry[20:]) != 512 {
		f.Close()
		return nil, fmt.Errorf("destination sector size verification failed: %v", err)
	}
	return f, nil
}
func Flush(f *os.File) error { return windows.FlushFileBuffers(windows.Handle(f.Fd())) }
func CheckSource(path string, d Drive) error {
	full, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(full)
	if len(volume) != 2 || volume[1] != ':' {
		return fmt.Errorf("use an image on a local drive with a drive letter")
	}
	letter := strings.ToUpper(volume[:1])
	if letter[0] < 'A' || letter[0] > 'Z' {
		return fmt.Errorf("invalid source volume")
	}
	b, err := ps("ConvertTo-Json -InputObject @(Get-Partition -DriveLetter " + letter + " | Select-Object DiskNumber)")
	if err != nil {
		return err
	}
	var parts []struct{ DiskNumber int }
	if err = json.Unmarshal(b, &parts); err != nil {
		return err
	}
	if len(parts) == 0 {
		return fmt.Errorf("cannot identify source image disk")
	}
	n, err := number(d)
	if err != nil {
		return err
	}
	for _, p := range parts {
		if p.DiskNumber == n {
			return fmt.Errorf("source image is on the destination disk")
		}
	}
	return nil
}
func Privileged() bool { return windows.GetCurrentProcessToken().IsElevated() }
