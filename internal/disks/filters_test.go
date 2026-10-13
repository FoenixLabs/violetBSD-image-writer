package disks

import (
	"encoding/json"
	"testing"
)

func TestLinuxExcludesInUseAndInternal(t *testing.T) {
	base := linuxDisk{Path: "/dev/sdz", Type: "disk", Size: 32e9, Tran: "usb", Sector: 512, MajorMinor: "8:240"}
	changes := []func(*linuxDisk){func(*linuxDisk) {}, func(d *linuxDisk) { d.Tran = "sata" }, func(d *linuxDisk) { d.RO = true }, func(d *linuxDisk) { s := "/"; d.Mounts = []*string{&s} }, func(d *linuxDisk) { s := "[SWAP]"; d.Children = []linuxDisk{{Type: "part", Mounts: []*string{&s}}} }, func(d *linuxDisk) { d.Children = []linuxDisk{{Type: "crypt"}} }, func(d *linuxDisk) { d.Sector = 4096 }}
	for i, change := range changes {
		d := base
		change(&d)
		b, _ := json.Marshal(struct{ Blockdevices []linuxDisk }{[]linuxDisk{d}})
		got, err := parseLinux(b)
		if err != nil {
			t.Fatal(err)
		}
		if (len(got) == 1) != (i == 0) {
			t.Fatalf("filter case %d", i)
		}
	}
	if _, err := parseLinux([]byte("bad")); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
func TestWindowsExcludesSystemAndUnsupported(t *testing.T) {
	base := winDisk{Number: 7, FriendlyName: "USB", UniqueId: "drive-123", Size: 32e9, BusType: 7, LogicalSectorSize: 512}
	changes := []func(*winDisk){func(*winDisk) {}, func(d *winDisk) { d.IsBoot = true }, func(d *winDisk) { d.IsSystem = true }, func(d *winDisk) { d.IsReadOnly = true }, func(d *winDisk) { d.BusType = 11 }, func(d *winDisk) { d.LogicalSectorSize = 4096 }, func(d *winDisk) { d.UniqueId = "" }}
	for i, change := range changes {
		d := base
		change(&d)
		b, _ := json.Marshal([]winDisk{d})
		got, err := parseWindows(b)
		if err != nil {
			t.Fatal(err)
		}
		if (len(got) == 1) != (i == 0) {
			t.Fatalf("filter case %d", i)
		}
	}
}
func TestMacExcludesMountedAPFSAndInternal(t *testing.T) {
	d := macDisk{DeviceIdentifier: "disk7"}
	i := macInfo{Whole: true, Writable: true, TotalSize: 32e9, DeviceBlockSize: 512, DeviceIdentifier: "disk7", DeviceTreePath: "external/usb"}
	if !macEligible(d, i) {
		t.Fatal("eligible external disk rejected")
	}
	for _, bad := range []macDisk{{DeviceIdentifier: "disk7", MountPoint: "/"}, {DeviceIdentifier: "disk7", Partitions: []macPart{{Content: "Apple_APFS"}}}, {DeviceIdentifier: "disk7s1"}, {DeviceIdentifier: "disk7", Partitions: []macPart{{MountPoint: "/Volumes/external"}}}} {
		if macEligible(bad, i) {
			t.Fatal("unsafe disk accepted")
		}
	}
	i.Internal = true
	if macEligible(d, i) {
		t.Fatal("internal disk accepted")
	}
}
