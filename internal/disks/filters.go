package disks

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

type linuxDisk struct {
	Path, Name, Type, Model, Serial, Tran string
	Size                                  int64
	RO                                    bool      `json:"ro"`
	RM                                    bool      `json:"rm"`
	MajorMinor                            string    `json:"maj:min"`
	Sector                                int64     `json:"log-sec"`
	Mounts                                []*string `json:"mountpoints"`
	Children                              []linuxDisk
}

func busy(d linuxDisk) bool {
	if d.Type != "disk" && d.Type != "part" {
		return true
	}
	for _, m := range d.Mounts {
		if m != nil && *m != "" {
			return true
		}
	}
	for _, c := range d.Children {
		if busy(c) {
			return true
		}
	}
	return false
}
func parseLinux(b []byte) ([]Drive, error) {
	var data struct{ Blockdevices []linuxDisk }
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	var result []Drive
	for _, d := range data.Blockdevices {
		if d.Type != "disk" || d.RO || busy(d) || d.Sector != 512 || d.Size <= 0 || !(d.RM || d.Tran == "usb" || d.Tran == "mmc") {
			continue
		}
		result = append(result, Drive{d.Path, strings.TrimSpace(d.Model), d.Serial, d.Size, d.MajorMinor})
	}
	return result, nil
}

type winDisk struct {
	Number                                  int
	FriendlyName, SerialNumber, UniqueId    string
	Size                                    int64
	IsBoot, IsSystem, IsReadOnly, IsOffline bool
	BusType                                 int
	LogicalSectorSize                       int
}

func parseWindows(b []byte) ([]Drive, error) {
	var data []winDisk
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	var result []Drive
	for _, d := range data {
		if d.IsBoot || d.IsSystem || d.IsReadOnly || d.Size <= 0 || d.LogicalSectorSize != 512 || d.UniqueId == "" || (d.BusType != 7 && d.BusType != 12 && d.BusType != 13) {
			continue
		}
		result = append(result, Drive{`\\.\PhysicalDrive` + strconv.Itoa(d.Number), d.FriendlyName, d.SerialNumber, d.Size, d.UniqueId})
	}
	return result, nil
}

type macPart struct{ MountPoint, Content string }
type macDisk struct {
	DeviceIdentifier    string
	Partitions          []macPart
	Content, MountPoint string
}
type macInfo struct {
	Internal, Whole, Writable, Mounted          bool
	TotalSize                                   int64
	DeviceBlockSize                             int64
	MediaName, DeviceTreePath, DeviceIdentifier string
}

var macID = regexp.MustCompile(`^disk[0-9]+$`)

func macEligible(d macDisk, i macInfo) bool {
	if !macID.MatchString(d.DeviceIdentifier) || i.DeviceIdentifier != d.DeviceIdentifier || i.Internal || !i.Whole || !i.Writable || i.Mounted || i.DeviceBlockSize != 512 || i.TotalSize <= 0 || i.DeviceTreePath == "" || d.MountPoint != "" || strings.Contains(d.Content, "APFS") {
		return false
	}
	for _, p := range d.Partitions {
		if p.MountPoint != "" || strings.Contains(p.Content, "APFS") {
			return false
		}
	}
	return true
}
