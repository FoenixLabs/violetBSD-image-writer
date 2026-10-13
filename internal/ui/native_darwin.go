package ui

import (
	"fmt"
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"runtime"
)

type point struct{ X, Y float64 }
type extent struct{ Width, Height float64 }
type rect struct {
	Origin point
	Size   extent
}

func rectangle(x, y, w, h float64) rect { return rect{point{x, y}, extent{w, h}} }
func sel(s string) objc.SEL             { return objc.RegisterName(s) }
func ns(s string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(sel("stringWithUTF8String:"), s)
}
func alloc(class string) objc.ID { return objc.ID(objc.GetClass(class)).Send(sel("alloc")) }
func runNative(c *Controller) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := purego.Dlopen("/System/Library/Frameworks/Cocoa.framework/Cocoa", purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		return err
	}
	pool := alloc("NSAutoreleasePool").Send(sel("init"))
	defer pool.Send(sel("drain"))
	app := objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication"))
	app.Send(sel("setActivationPolicy:"), 0)
	win := alloc("NSWindow").Send(sel("initWithContentRect:styleMask:backing:defer:"), rectangle(0, 0, 680, 320), uintptr(3), uintptr(2), false)
	win.Send(sel("setTitle:"), ns(Title))
	win.Send(sel("setReleasedWhenClosed:"), false)
	content := win.Send(sel("contentView"))
	label := func(text string, r rect) objc.ID {
		v := alloc("NSTextField").Send(sel("initWithFrame:"), r)
		v.Send(sel("setStringValue:"), ns(text))
		v.Send(sel("setEditable:"), false)
		v.Send(sel("setBezeled:"), false)
		v.Send(sel("setDrawsBackground:"), false)
		content.Send(sel("addSubview:"), v)
		return v
	}
	label(Title, rectangle(20, 275, 640, 26))
	image := label("Select a .img or .img.gz image", rectangle(20, 225, 480, 26))
	var imagePath string
	button := func(text string, r rect) objc.ID {
		v := alloc("NSButton").Send(sel("initWithFrame:"), r)
		v.Send(sel("setTitle:"), ns(text))
		v.Send(sel("setBezelStyle:"), uintptr(1))
		content.Send(sel("addSubview:"), v)
		return v
	}
	browse := button("Select image…", rectangle(510, 221, 150, 32))
	combo := alloc("NSPopUpButton").Send(sel("initWithFrame:pullsDown:"), rectangle(20, 173, 480, 32), false)
	content.Send(sel("addSubview:"), combo)
	refresh := button("Refresh drives", rectangle(510, 173, 150, 32))
	status := label("", rectangle(20, 84, 640, 76))
	status.Send(sel("cell")).Send(sel("setWraps:"), true)
	progress := alloc("NSProgressIndicator").Send(sel("initWithFrame:"), rectangle(20, 59, 640, 16))
	progress.Send(sel("setIndeterminate:"), false)
	progress.Send(sel("setMinValue:"), float64(0))
	progress.Send(sel("setMaxValue:"), float64(1))
	content.Send(sel("addSubview:"), progress)
	write := button("Write image", rectangle(510, 14, 150, 32))
	message := func(text string, confirm bool) bool {
		alert := alloc("NSAlert").Send(sel("init"))
		defer alert.Send(sel("release"))
		alert.Send(sel("setMessageText:"), ns(Title))
		alert.Send(sel("setInformativeText:"), ns(text))
		alert.Send(sel("addButtonWithTitle:"), ns("Cancel"))
		if confirm {
			alert.Send(sel("addButtonWithTitle:"), ns("Erase and write"))
		}
		return objc.Send[int64](alert, sel("runModal")) == 1001
	}
	refreshList := func() {
		if err := c.Refresh(); err != nil {
			c.report(err.Error(), 0)
		}
		combo.Send(sel("removeAllItems"))
		for _, d := range c.Drives {
			combo.Send(sel("addItemWithTitle:"), ns(d.Label()))
		}
		combo.Send(sel("selectItemAtIndex:"), int64(-1))
	}
	methods := []objc.MethodDef{
		{Cmd: sel("browse:"), Fn: func(objc.ID, objc.SEL, objc.ID) {
			panel := objc.ID(objc.GetClass("NSOpenPanel")).Send(sel("openPanel"))
			panel.Send(sel("setCanChooseDirectories:"), false)
			panel.Send(sel("setAllowsMultipleSelection:"), false)
			if objc.Send[int64](panel, sel("runModal")) == 1 {
				path := panel.Send(sel("URL")).Send(sel("path"))
				imagePath = objc.Send[string](path, sel("UTF8String"))
				image.Send(sel("setStringValue:"), path)
			}
		}},
		{Cmd: sel("refresh:"), Fn: func(objc.ID, objc.SEL, objc.ID) { refreshList() }},
		{Cmd: sel("write:"), Fn: func(objc.ID, objc.SEL, objc.ID) {
			index := objc.Send[int64](combo, sel("indexOfSelectedItem"))
			d, err := c.Validate(imagePath, int(index))
			if err != nil {
				message(err.Error(), false)
				return
			}
			if message(fmt.Sprintf("Erase ALL data on:\n%s\nSerial: %s\n\nWrite %s?", d.Label(), d.Serial, imagePath), true) {
				c.Start(imagePath, d)
			}
		}},
		{Cmd: sel("tick:"), Fn: func(objc.ID, objc.SEL, objc.ID) {
			busy, text, fraction := c.Snapshot()
			status.Send(sel("setStringValue:"), ns(text))
			progress.Send(sel("setDoubleValue:"), fraction)
			for _, v := range []objc.ID{browse, combo, refresh, write} {
				v.Send(sel("setEnabled:"), !busy)
			}
		}},
		{Cmd: sel("windowShouldClose:"), Fn: func(objc.ID, objc.SEL, objc.ID) bool {
			busy, _, _ := c.Snapshot()
			if busy {
				message("Wait for writing and verification to finish.", false)
				return false
			}
			app.Send(sel("stop:"), objc.ID(0))
			return true
		}},
		{Cmd: sel("applicationShouldTerminate:"), Fn: func(objc.ID, objc.SEL, objc.ID) uintptr {
			busy, _, _ := c.Snapshot()
			if busy {
				message("Wait for writing and verification to finish.", false)
				return 0
			}
			return 1
		}},
	}
	class, err := objc.RegisterClass("VioletWriterDelegate", objc.GetClass("NSObject"), nil, nil, methods)
	if err != nil {
		return err
	}
	delegate := objc.ID(class).Send(sel("new"))
	defer delegate.Send(sel("release"))
	win.Send(sel("setDelegate:"), delegate)
	app.Send(sel("setDelegate:"), delegate)
	for _, pair := range []struct {
		v      objc.ID
		action string
	}{{browse, "browse:"}, {refresh, "refresh:"}, {write, "write:"}} {
		pair.v.Send(sel("setTarget:"), delegate)
		pair.v.Send(sel("setAction:"), sel(pair.action))
	}
	timer := objc.ID(objc.GetClass("NSTimer")).Send(sel("scheduledTimerWithTimeInterval:target:selector:userInfo:repeats:"), float64(.15), delegate, sel("tick:"), objc.ID(0), true)
	defer timer.Send(sel("invalidate"))
	refreshList()
	win.Send(sel("center"))
	win.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	app.Send(sel("activateIgnoringOtherApps:"), true)
	app.Send(sel("run"))
	return nil
}
