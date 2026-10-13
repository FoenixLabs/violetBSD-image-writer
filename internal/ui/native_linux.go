package ui

import (
	"fmt"
	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
	"runtime"
)

// Only the small GTK3 API used by this window is bound. GTK remains a system
// dependency; no Python, browser, shared application library or C compiler is used.
func runNative(c *Controller) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	lib, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("GTK 3 is required: %w", err)
	}
	bind := func(name string, f any) { purego.RegisterLibFunc(f, lib, name) }
	var init func(uintptr, uintptr) int32
	bind("gtk_init_check", &init)
	if init(0, 0) == 0 {
		return fmt.Errorf("cannot connect to the desktop display")
	}
	var windowNew func(int32) uintptr
	bind("gtk_window_new", &windowNew)
	var title func(uintptr, string)
	bind("gtk_window_set_title", &title)
	var size func(uintptr, int32, int32)
	bind("gtk_window_set_default_size", &size)
	var border func(uintptr, uint32)
	bind("gtk_container_set_border_width", &border)
	var boxNew func(int32, int32) uintptr
	bind("gtk_box_new", &boxNew)
	var pack func(uintptr, uintptr, int32, int32, uint32)
	bind("gtk_box_pack_start", &pack)
	var add func(uintptr, uintptr)
	bind("gtk_container_add", &add)
	var labelNew func(string) uintptr
	bind("gtk_label_new", &labelNew)
	var labelSet func(uintptr, string)
	bind("gtk_label_set_text", &labelSet)
	var wrap func(uintptr, int32)
	bind("gtk_label_set_line_wrap", &wrap)
	var align func(uintptr, float32)
	bind("gtk_label_set_xalign", &align)
	var button func(string) uintptr
	bind("gtk_button_new_with_label", &button)
	var entryNew func() uintptr
	bind("gtk_entry_new", &entryNew)
	var entryGet func(uintptr) string
	bind("gtk_entry_get_text", &entryGet)
	var entrySet func(uintptr, string)
	bind("gtk_entry_set_text", &entrySet)
	var comboNew func() uintptr
	bind("gtk_combo_box_text_new", &comboNew)
	var comboAdd func(uintptr, string)
	bind("gtk_combo_box_text_append_text", &comboAdd)
	var comboClear func(uintptr)
	bind("gtk_combo_box_text_remove_all", &comboClear)
	var selected func(uintptr) int32
	bind("gtk_combo_box_get_active", &selected)
	var barNew func() uintptr
	bind("gtk_progress_bar_new", &barNew)
	var fraction func(uintptr, float64)
	bind("gtk_progress_bar_set_fraction", &fraction)
	var sensitive func(uintptr, int32)
	bind("gtk_widget_set_sensitive", &sensitive)
	var show func(uintptr)
	bind("gtk_widget_show_all", &show)
	var destroy func(uintptr)
	bind("gtk_widget_destroy", &destroy)
	var main func()
	bind("gtk_main", &main)
	var quit func()
	bind("gtk_main_quit", &quit)
	var connect func(uintptr, string, uintptr, uintptr, uintptr, uint32) uint64
	bind("g_signal_connect_data", &connect)
	var timer func(uint32, uintptr, uintptr) uint32
	bind("g_timeout_add", &timer)
	var removeTimer func(uint32) int32
	bind("g_source_remove", &removeTimer)
	var dialogNew func() uintptr
	bind("gtk_dialog_new", &dialogNew)
	var dialogArea func(uintptr) uintptr
	bind("gtk_dialog_get_content_area", &dialogArea)
	var dialogButton func(uintptr, string, int32) uintptr
	bind("gtk_dialog_add_button", &dialogButton)
	var dialogDefault func(uintptr, int32)
	bind("gtk_dialog_set_default_response", &dialogDefault)
	var transient func(uintptr, uintptr)
	bind("gtk_window_set_transient_for", &transient)
	var modal func(uintptr, int32)
	bind("gtk_window_set_modal", &modal)
	var runDialog func(uintptr) int32
	bind("gtk_dialog_run", &runDialog)
	var fileNew func(string, uintptr, int32, string, int32, string, int32, uintptr) uintptr
	bind("gtk_file_chooser_dialog_new", &fileNew)
	var filename func(uintptr) *byte
	bind("gtk_file_chooser_get_filename", &filename)
	var free func(*byte)
	bind("g_free", &free)
	win := windowNew(0)
	title(win, Title)
	size(win, 660, 330)
	border(win, 20)
	box := boxNew(1, 12)
	add(win, box)
	heading := labelNew(Title)
	align(heading, 0)
	pack(box, heading, 0, 0, 0)
	row := boxNew(0, 8)
	pack(box, row, 0, 0, 0)
	entry := entryNew()
	var placeholder func(uintptr, string)
	bind("gtk_entry_set_placeholder_text", &placeholder)
	placeholder(entry, "Choose a .img or .img.gz image")
	pack(row, entry, 1, 1, 0)
	browse := button("Select image…")
	pack(row, browse, 0, 0, 0)
	row2 := boxNew(0, 8)
	pack(box, row2, 0, 0, 0)
	combo := comboNew()
	pack(row2, combo, 1, 1, 0)
	refresh := button("Refresh drives")
	pack(row2, refresh, 0, 0, 0)
	status := labelNew("")
	wrap(status, 1)
	align(status, 0)
	pack(box, status, 1, 1, 0)
	progress := barNew()
	pack(box, progress, 0, 0, 0)
	write := button("Write image")
	pack(box, write, 0, 0, 0)
	message := func(text string, confirm bool) bool {
		dlg := dialogNew()
		title(dlg, Title)
		transient(dlg, win)
		modal(dlg, 1)
		border(dlg, 16)
		l := labelNew(text)
		wrap(l, 1)
		add(dialogArea(dlg), l)
		dialogButton(dlg, "Cancel", -6)
		if confirm {
			dialogButton(dlg, "Erase and write", -3)
		}
		dialogDefault(dlg, -6)
		show(dlg)
		response := runDialog(dlg)
		destroy(dlg)
		return response == -3
	}
	refreshList := func() {
		if err := c.Refresh(); err != nil {
			c.report(err.Error(), 0)
		}
		comboClear(combo)
		for _, d := range c.Drives {
			comboAdd(combo, d.Label())
		}
	}
	connect(refresh, "clicked", purego.NewCallback(func(uintptr, uintptr) { refreshList() }), 0, 0, 0)
	connect(browse, "clicked", purego.NewCallback(func(uintptr, uintptr) {
		dlg := fileNew("Select violetBSD .img or .img.gz", win, 0, "Cancel", -6, "Select", -3, 0)
		if runDialog(dlg) == -3 {
			p := filename(dlg)
			if p != nil {
				entrySet(entry, unix.BytePtrToString(p))
				free(p)
			}
		}
		destroy(dlg)
	}), 0, 0, 0)
	connect(write, "clicked", purego.NewCallback(func(uintptr, uintptr) {
		path := entryGet(entry)
		d, err := c.Validate(path, int(selected(combo)))
		if err != nil {
			message(err.Error(), false)
			return
		}
		if message("Erase ALL data on:\n"+d.Label()+"\nSerial: "+d.Serial+"\n\nWrite "+path+"?", true) {
			c.Start(path, d)
		}
	}), 0, 0, 0)
	connect(win, "delete-event", purego.NewCallback(func(uintptr, uintptr, uintptr) int32 {
		busy, _, _ := c.Snapshot()
		if busy {
			message("Wait for writing and verification to finish.", false)
			return 1
		}
		quit()
		return 0
	}), 0, 0, 0)
	refreshList()
	tick := timer(150, purego.NewCallback(func(uintptr) int32 {
		busy, text, p := c.Snapshot()
		labelSet(status, text)
		fraction(progress, p)
		enabled := int32(1)
		if busy {
			enabled = 0
		}
		for _, w := range []uintptr{entry, browse, combo, refresh, write} {
			sensitive(w, enabled)
		}
		return 1
	}), 0)
	defer removeTimer(tick)
	show(win)
	main()
	return nil
}
