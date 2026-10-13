package ui

import (
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

var user = windows.NewLazySystemDLL("user32.dll")
var gdi = windows.NewLazySystemDLL("gdi32.dll")

func proc(name string) *windows.LazyProc { return user.NewProc(name) }
func u16(s string) *uint16               { p, _ := windows.UTF16PtrFromString(s); return p }
func ptr(p *uint16) uintptr              { return uintptr(unsafe.Pointer(p)) }

type winClass struct {
	Size, Style                        uint32
	Callback                           uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type winPoint struct{ X, Y int32 }
type winMsg struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          winPoint
	Private        uint32
}
type openFile struct {
	Size                   uint32
	Owner, Instance        uintptr
	Filter, CustomFilter   *uint16
	MaxCustom, FilterIndex uint32
	File                   *uint16
	MaxFile                uint32
	FileTitle              *uint16
	MaxTitle               uint32
	InitialDir, Title      *uint16
	Flags                  uint32
	FileOffset, Extension  uint16
	DefaultExtension       *uint16
	CustomData, Hook       uintptr
	Template               *uint16
	Reserved               uintptr
	Reserved2, FlagsEx     uint32
}

func runNative(c *Controller) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	create := proc("CreateWindowExW")
	send := proc("SendMessageW")
	setText := proc("SetWindowTextW")
	enable := proc("EnableWindow")
	def := proc("DefWindowProcW")
	destroy := proc("DestroyWindow")
	quit := proc("PostQuitMessage")
	var win, image, combo, status, progress, browse, refresh, write uintptr
	var imagePath string
	text := func(w uintptr, s string) { setText.Call(w, ptr(u16(s))) }
	message := func(s string, confirm bool) bool {
		flags := uintptr(0x30)
		if confirm {
			flags |= 0x4 | 0x100
		} // YES/NO, default NO.
		result, _, _ := proc("MessageBoxW").Call(win, ptr(u16(s)), ptr(u16(Title)), flags)
		return result == 6
	}
	refreshList := func() {
		if err := c.Refresh(); err != nil {
			c.report(err.Error(), 0)
		}
		send.Call(combo, 0x14b, 0, 0) // CB_RESETCONTENT
		for _, d := range c.Drives {
			send.Call(combo, 0x143, 0, ptr(u16(d.Label())))
		}
		send.Call(combo, 0x14e, ^uintptr(0), 0) // CB_SETCURSEL -1
	}
	callback := windows.NewCallback(func(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
		switch msg {
		case 0x111: // WM_COMMAND
			switch wp & 0xffff {
			case 101:
				buf := make([]uint16, 32768)
				filter := []uint16{'D', 'i', 's', 'k', ' ', 'i', 'm', 'a', 'g', 'e', 's', 0, '*', '.', 'i', 'm', 'g', ';', '*', '.', 'i', 'm', 'g', '.', 'g', 'z', 0, 'A', 'l', 'l', ' ', 'f', 'i', 'l', 'e', 's', 0, '*', '.', '*', 0, 0}
				ofn := openFile{Owner: win, Filter: &filter[0], File: &buf[0], MaxFile: uint32(len(buf)), Title: u16("Select violetBSD image"), Flags: 0x1000 | 0x800 | 0x8 | 0x80000}
				ofn.Size = uint32(unsafe.Sizeof(ofn))
				ok, _, _ := windows.NewLazySystemDLL("comdlg32.dll").NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&ofn)))
				if ok != 0 {
					imagePath = windows.UTF16ToString(buf)
					text(image, imagePath)
				}
			case 102:
				refreshList()
			case 103:
				index, _, _ := send.Call(combo, 0x147, 0, 0)
				d, err := c.Validate(imagePath, int(index))
				if err != nil {
					message(err.Error(), false)
					return 0
				}
				if message(fmt.Sprintf("Erase ALL data on:\n%s\nSerial: %s\n\nWrite %s?", d.Label(), d.Serial, imagePath), true) {
					c.Start(imagePath, d)
				}
			}
			return 0
		case 0x113: // WM_TIMER
			busy, s, fraction := c.Snapshot()
			text(status, s)
			send.Call(progress, 0x402, uintptr(fraction*1000), 0)
			flag := uintptr(1)
			if busy {
				flag = 0
			}
			for _, v := range []uintptr{browse, combo, refresh, write} {
				enable.Call(v, flag)
			}
			return 0
		case 0x10: // WM_CLOSE
			if busy, _, _ := c.Snapshot(); busy {
				message("Wait for writing and verification to finish.", false)
				return 0
			}
			destroy.Call(hwnd)
			return 0
		case 2:
			quit.Call(0)
			return 0
		}
		result, _, _ := def.Call(hwnd, uintptr(msg), wp, lp)
		return result
	})
	var instance windows.Handle
	err := windows.GetModuleHandleEx(0, nil, &instance)
	if err != nil {
		return err
	}
	cursor, _, _ := proc("LoadCursorW").Call(0, 32512)
	class := winClass{Callback: callback, Instance: uintptr(instance), Cursor: cursor, Background: 6, Name: u16("VioletImageWriter")}
	class.Size = uint32(unsafe.Sizeof(class))
	registered, _, err := proc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if registered == 0 {
		return fmt.Errorf("register window: %w", err)
	}
	win, _, err = create.Call(0, ptr(class.Name), ptr(u16(Title)), 0x00ca0000, 0x80000000, 0x80000000, 740, 390, 0, 0, uintptr(instance), 0)
	if win == 0 {
		return fmt.Errorf("create window: %w", err)
	}
	font, _, _ := gdi.NewProc("GetStockObject").Call(17)
	control := func(kind, label string, x, y, w, h int32, style, id uintptr) uintptr {
		hwnd, _, _ := create.Call(0, ptr(u16(kind)), ptr(u16(label)), 0x50000000|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), win, id, uintptr(instance), 0)
		send.Call(hwnd, 0x30, font, 1)
		return hwnd
	}
	control("STATIC", Title, 22, 20, 650, 28, 0, 0)
	image = control("EDIT", "Select a .img or .img.gz image", 22, 62, 500, 27, 0x800|0x80|0x800000, 0)
	browse = control("BUTTON", "Select image…", 538, 60, 160, 30, 0x10000, 101)
	combo = control("COMBOBOX", "", 22, 110, 500, 230, 3|0x200000|0x10000, 104)
	refresh = control("BUTTON", "Refresh drives", 538, 108, 160, 30, 0x10000, 102)
	status = control("STATIC", "", 22, 158, 676, 95, 0, 0)
	initControls := windows.NewLazySystemDLL("comctl32.dll").NewProc("InitCommonControlsEx")
	icc := struct{ Size, Flags uint32 }{8, 0x20}
	initControls.Call(uintptr(unsafe.Pointer(&icc)))
	progress = control("msctls_progress32", "", 22, 265, 676, 20, 0, 0)
	send.Call(progress, 0x406, 0, 1000)
	write = control("BUTTON", "Write image", 538, 303, 160, 30, 0x10000, 103)
	refreshList()
	proc("SetTimer").Call(win, 1, 150, 0)
	defer proc("KillTimer").Call(win, 1)
	proc("ShowWindow").Call(win, 1)
	proc("UpdateWindow").Call(win)
	var m winMsg
	for {
		ret, _, err := proc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) == -1 {
			return err
		}
		if ret == 0 {
			break
		}
		proc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		proc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}
