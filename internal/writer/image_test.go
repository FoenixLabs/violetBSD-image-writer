package writer

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"io"
	"os"
	"testing"
)

func fixture() ([]byte, int64) {
	b := make([]byte, 2*1024*1024)
	copy(b[510:], []byte{0x55, 0xaa})
	b[446+4] = 0xa9
	binary.LittleEndian.PutUint32(b[446+8:], 2048)
	binary.LittleEndian.PutUint32(b[446+12:], uint32(len(b)/512-2048))
	put := func(off int, v uint32) { be.PutUint32(b[off:], v) }
	label := 2049 * 512
	root := 2050 * 512
	put(label, 0x82564557)
	put(label+132, 0x82564557)
	put(label+40, 512)
	be.PutUint16(b[label+138:], 4)
	b[label+148+12] = 7
	put(label+148+2*16, uint32(len(b)/512-2048))
	put(label+152+2*16, 2048)
	put(label+148+3*16, uint32(len(b)/512))
	put(label+148, uint32((len(b)-root)/512))
	put(label+152, 2050)
	sb := root + 8192
	for off, v := range map[int]uint32{1372: 0x11954, 48: 8192, 52: 1024, 184: 64, 188: 1024, 44: 1, 16: 16} {
		put(sb+off, v)
	}
	for idx, name := range []string{"var", "db", "entropy-file"} {
		inode := root + 16*1024 + (idx+2)*128
		be.PutUint16(b[inode:], 0040755)
		be.PutUint64(b[inode+8:], 512)
		put(inode+40, uint32(24+idx))
		entry := root + (24+idx)*1024
		put(entry, uint32(idx+3))
		be.PutUint16(b[entry+4:], 512)
		b[entry+7] = byte(len(name))
		copy(b[entry+8:], name)
	}
	inode := root + 16*1024 + 5*128
	be.PutUint16(b[inode:], 0100600)
	be.PutUint16(b[inode+2:], 1)
	be.PutUint64(b[inode+8:], SeedSize)
	put(inode+40, 27)
	off := int64(root + 27*1024)
	sum := sha1.Sum(b[off : off+516])
	copy(b[off+516:], sum[:])
	fixLabelChecksum(b[label : label+512])
	return b, off
}
func fixLabelChecksum(label []byte) {
	be.PutUint16(label[136:], 0)
	var sum uint16
	for i := 0; i < 148+16*int(be.Uint16(label[138:])); i += 2 {
		sum ^= be.Uint16(label[i:])
	}
	be.PutUint16(label[136:], sum)
}

func multiBootFixture() ([]byte, int64) {
	old, offset := fixture()
	const start = 8192
	const delta = (start - 2048) * 512
	b := make([]byte, len(old)+delta)
	copy(b[:512], old[:512])
	copy(b[start*512:], old[2048*512:])
	for i, p := range []struct {
		kind         byte
		start, count uint32
	}{{6, 2048, 2048}, {0x0c, 4096, 4096}, {0xa9, start, uint32(len(old)/512 - 2048)}} {
		entry := b[446+16*i : 462+16*i]
		clear(entry)
		entry[4] = p.kind
		binary.LittleEndian.PutUint32(entry[8:], p.start)
		binary.LittleEndian.PutUint32(entry[12:], p.count)
	}
	label := b[(start+1)*512 : (start+2)*512]
	be.PutUint32(label[152:], start+2)
	be.PutUint32(label[152+2*16:], start)
	be.PutUint32(label[148+3*16:], uint32(len(b)/512))
	fixLabelChecksum(label)
	return b, offset + delta
}

func TestMultiBootSeedOffset(t *testing.T) {
	b, want := multiBootFixture()
	got, err := SeedOffset(bytes.NewReader(b), int64(len(b)))
	if err != nil || got != want {
		t.Fatalf("got %d, want %d: %v", got, want, err)
	}
}

func TestRejectInvalidPartitionMapping(t *testing.T) {
	for name, damage := range map[string]func([]byte){
		"missing NetBSD":     func(b []byte) { b[446+2*16+4] = 0x0c },
		"overlap":            func(b []byte) { binary.LittleEndian.PutUint32(b[446+16+8:], 2048) },
		"out of bounds FAT":  func(b []byte) { binary.LittleEndian.PutUint32(b[446+12:], 0xffffffff) },
		"zero length":        func(b []byte) { binary.LittleEndian.PutUint32(b[446+2*16+12:], 0) },
		"duplicate NetBSD":   func(b []byte) { b[446+4] = 0xa9 },
		"bad label count":    func(b []byte) { be.PutUint16(b[8193*512+138:], 17) },
		"bad label checksum": func(b []byte) { b[8193*512+136] ^= 1 },
		"root in FAT": func(b []byte) {
			label := b[8193*512 : 8194*512]
			be.PutUint32(label[152:], 4096)
			fixLabelChecksum(label)
		},
		"container mismatch": func(b []byte) {
			label := b[8193*512 : 8194*512]
			be.PutUint32(label[152+2*16:], 2048)
			fixLabelChecksum(label)
		},
	} {
		t.Run(name, func(t *testing.T) {
			b, _ := multiBootFixture()
			damage(b)
			if _, err := SeedOffset(bytes.NewReader(b), int64(len(b))); err == nil {
				t.Fatal("accepted invalid mapping")
			}
		})
	}
}

func TestSeedFormatAndFreshness(t *testing.T) {
	a, err := Seed()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Seed()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != SeedSize || binary.BigEndian.Uint32(a) != 256 || bytes.Equal(a, b) {
		t.Fatal("bad or reused seed")
	}
	sum := sha1.Sum(a[:516])
	if !bytes.Equal(sum[:], a[516:]) {
		t.Fatal("checksum")
	}
}
func TestSeedOffset(t *testing.T) {
	b, want := fixture()
	got, err := SeedOffset(bytes.NewReader(b), int64(len(b)))
	if err != nil || got != want {
		t.Fatalf("%d %v", got, err)
	}
}
func TestRejectDamagedImages(t *testing.T) {
	tests := map[string]func([]byte){
		"MBR":         func(b []byte) { b[510] = 0 },
		"label":       func(b []byte) { b[2049*512] = 0 },
		"geometry":    func(b []byte) { be.PutUint32(b[2050*512+8192+48:], 4096) },
		"outside":     func(b []byte) { be.PutUint32(b[2049*512+148:], 0xffffffff) },
		"permissions": func(b []byte) { be.PutUint16(b[2050*512+16*1024+5*128:], 0100644) },
		"hardlink":    func(b []byte) { be.PutUint16(b[2050*512+16*1024+5*128+2:], 2) },
		"indirect":    func(b []byte) { b[2050*512+16*1024+5*128+44] = 1 },
		"directory":   func(b []byte) { be.PutUint16(b[2050*512+24*1024+4:], 0) },
		"checksum":    func(b []byte) { b[2050*512+27*1024+4] = 1 },
	}
	for name, damage := range tests {
		t.Run(name, func(t *testing.T) {
			b, _ := fixture()
			damage(b)
			if _, err := SeedOffset(bytes.NewReader(b), int64(len(b))); err == nil {
				t.Fatal("accepted damaged image")
			}
		})
	}
	b, _ := fixture()
	for _, size := range []int64{0, 511, 2049*512 + 128, 2050*512 + 8200} {
		if _, err := SeedOffset(bytes.NewReader(b[:size]), size); err == nil {
			t.Fatal("accepted truncation")
		}
	}
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(b []byte) (int, error) { return w.Buffer.Write(b[:min(len(b), 113)]) }

type stoppedWriter struct{}

func (stoppedWriter) Write([]byte) (int, error) { return 0, nil }
func TestPersonalizedRoundTrip(t *testing.T) {
	src := bytes.Repeat([]byte{0x75}, Chunk+1024)
	before := bytes.Clone(src)
	seed, err := Seed()
	if err != nil {
		t.Fatal(err)
	}
	offset := int64(Chunk - 128)
	var dst shortWriter
	digest, err := Copy(&dst, bytes.NewReader(src), int64(len(src)), offset, seed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(src, before) || !bytes.Equal(dst.Bytes()[:offset], src[:offset]) || !bytes.Equal(dst.Bytes()[offset+SeedSize:], src[offset+SeedSize:]) || !bytes.Equal(dst.Bytes()[offset:offset+SeedSize], seed) {
		t.Fatal("unexpected byte modification")
	}
	if err = Verify(bytes.NewReader(dst.Bytes()), int64(dst.Len()), digest, nil); err != nil {
		t.Fatal(err)
	}
	dst.Bytes()[4] ^= 1
	if err = Verify(bytes.NewReader(dst.Bytes()), int64(dst.Len()), digest, nil); err == nil {
		t.Fatal("accepted corrupted readback")
	}
	if err = Verify(bytes.NewReader(dst.Bytes()[:20]), int64(dst.Len()), digest, nil); err == nil {
		t.Fatal("accepted truncated readback")
	}
	if _, err = Copy(stoppedWriter{}, bytes.NewReader(src), int64(len(src)), offset, seed, nil); err != io.ErrShortWrite {
		t.Fatalf("short write: %v", err)
	}
	if _, err = Copy(io.Discard, bytes.NewReader(src[:20]), int64(len(src)), offset, seed, nil); err == nil {
		t.Fatal("accepted short image")
	}
}
func TestRealReleaseImage(t *testing.T) {
	path := os.Getenv("VIOLET_TEST_IMAGE")
	if path == "" {
		t.Skip("set VIOLET_TEST_IMAGE for read-only release-image integration")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	off, err := SeedOffset(f, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("validated real UFS seed extent at byte %d", off)
	b := make([]byte, SeedSize)
	if _, err = f.ReadAt(b, off); err != nil {
		t.Fatal(err)
	}
	if be.Uint32(b) != 0 {
		t.Fatal("public release must contain zero credited bits")
	}
}
func FuzzSeedOffset(f *testing.F) {
	b, _ := fixture()
	f.Add(b)
	f.Add([]byte("bad"))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = SeedOffset(bytes.NewReader(b), int64(len(b))) })
}
