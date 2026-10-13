// Build the three single-file release artifacts from any Go build host.
package main

import (
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func run(env []string, args ...string) {
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())
}
func main() {
	out := flag.String("out", "dist", "output directory")
	flag.Parse()
	must(os.MkdirAll(*out, 0755))
	run(nil, "run", "github.com/akavel/rsrc@v0.10.2", "-arch", "amd64", "-manifest", "windows.manifest", "-o", "rsrc_windows_amd64.syso")
	temp, err := os.MkdirTemp("", "violet-writer-build-")
	must(err)
	defer os.RemoveAll(temp)
	names := []string{"violetbsd-image-writer-linux-x86_64", "violetbsd-image-writer-windows-x86_64.exe", "violetbsd-image-writer-macos-universal"}
	for _, target := range []struct{ os, arch, path string }{{"linux", "amd64", filepath.Join(*out, names[0])}, {"windows", "amd64", filepath.Join(*out, names[1])}, {"darwin", "amd64", filepath.Join(temp, "mac-amd64")}, {"darwin", "arm64", filepath.Join(temp, "mac-arm64")}} {
		flags := "-s -w"
		if target.os == "windows" {
			flags += " -H=windowsgui"
		}
		run([]string{"CGO_ENABLED=0", "GOOS=" + target.os, "GOARCH=" + target.arch}, "build", "-trimpath", "-buildvcs=false", "-ldflags="+flags, "-o", target.path, ".")
	}
	must(universal(filepath.Join(*out, names[2]), filepath.Join(temp, "mac-amd64"), filepath.Join(temp, "mac-arm64")))
	var checksums string
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(*out, name))
		must(err)
		checksums += fmt.Sprintf("%x  %s\n", sha256.Sum256(b), name)
		fmt.Printf("%s: %.1f MiB\n", name, float64(len(b))/(1024*1024))
	}
	must(os.WriteFile(filepath.Join(*out, "violetbsd-image-writer.SHA256"), []byte(checksums), 0644))
}

// FAT_MAGIC header and two fat_arch entries. Each signed Mach-O slice is kept
// byte-for-byte intact and starts on a 16 KiB boundary, including its signature.
func universal(path, x64, arm string) error {
	paths := []string{x64, arm}
	data := make([][]byte, 2)
	offsets := make([]uint32, 2)
	position := uint32(16384)
	for i, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		data[i] = b
		offsets[i] = position
		position = (position + uint32(len(b)) + 16383) &^ 16383
	}
	result := make([]byte, position)
	be := binary.BigEndian
	be.PutUint32(result, 0xcafebabe)
	be.PutUint32(result[4:], 2)
	for i, b := range data {
		start := 8 + 20*i
		cpu, sub := uint32(0x01000007), uint32(3)
		if i == 1 {
			cpu, sub = 0x0100000c, 0
		}
		for j, v := range []uint32{cpu, sub, offsets[i], uint32(len(b)), 14} {
			be.PutUint32(result[start+4*j:], v)
		}
		copy(result[offsets[i]:], b)
	}
	if err := os.WriteFile(path, result, 0755); err != nil {
		return err
	}
	f, err := macho.OpenFat(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(f.Arches) != 2 || f.Arches[0].Cpu != macho.CpuAmd64 || f.Arches[1].Cpu != macho.CpuArm64 {
		return fmt.Errorf("invalid universal executable")
	}
	return nil
}
