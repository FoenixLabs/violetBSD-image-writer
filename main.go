// Copyright (c) 2026 violetBSD contributors. All rights reserved.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"runtime"
	"violetbsd.org/image-writer/internal/disks"
	"violetbsd.org/image-writer/internal/ui"
	"violetbsd.org/image-writer/internal/writer"
)

//go:embed THIRD_PARTY.txt
var thirdParty string

func init() { runtime.LockOSThread() }
func main() {
	inspect := flag.String("inspect", "", "validate a release image without writing")
	list := flag.Bool("list", false, "list eligible destinations without writing")
	version := flag.Bool("version", false, "print version")
	licenses := flag.Bool("licenses", false, "print bundled third-party notices")
	helper := flag.String("elevated-write", "", "internal administrator-authorized worker")
	flag.Parse()
	var err error
	switch {
	case *helper != "":
		err = ui.ElevatedWrite(*helper)
	case *licenses:
		fmt.Print(thirdParty)
		return
	case *version:
		fmt.Println("violetBSD Image Writer 0.2.3 (Go)")
		return
	case *inspect != "":
		var f *os.File
		f, err = os.Open(*inspect)
		if err == nil {
			defer f.Close()
			var info os.FileInfo
			info, err = disks.CheckImageFile(f)
			if err == nil {
				_, err = writer.SeedOffset(f, info.Size())
			}
		}
		if err == nil {
			fmt.Println("Compatible image; automatic per-device entropy provisioning available.")
		}
	case *list:
		var ds []disks.Drive
		ds, err = disks.Discover()
		if err == nil {
			for _, d := range ds {
				fmt.Println(d.Label())
			}
		}
	default:
		err = ui.Run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
