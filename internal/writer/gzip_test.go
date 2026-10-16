package writer

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"io"
	"os"
	"testing"
)

func compressedFixture(t *testing.T, b []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	g := gzip.NewWriter(&out)
	if _, e := g.Write(b); e != nil {
		t.Fatal(e)
	}
	if e := g.Close(); e != nil {
		t.Fatal(e)
	}
	return out.Bytes()
}
func TestGzipStreamPersonalization(t *testing.T) {
	t.Run("standard", func(t *testing.T) { raw, off := fixture(); testGzipPersonalization(t, raw, off) })
	t.Run("multi-boot", func(t *testing.T) { raw, off := multiBootFixture(); testGzipPersonalization(t, raw, off) })
}
func testGzipPersonalization(t *testing.T, raw []byte, wantOffset int64) {
	data := compressedFixture(t, raw)
	size, off, hash, err := InspectGzip(bytes.NewReader(data), int64(len(raw)))
	if err != nil || size != int64(len(raw)) || off != wantOffset || hash != sha256.Sum256(raw) {
		t.Fatalf("inspect: %d %d %v", size, off, err)
	}
	seed, _ := Seed()
	g, _ := gzip.NewReader(bytes.NewReader(data))
	defer g.Close()
	var out bytes.Buffer
	digest, err := Copy(&out, g, size, off, seed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if extra, e := io.ReadAll(g); e != nil || len(extra) != 0 {
		t.Fatalf("gzip EOF: %v", e)
	}
	want := append([]byte(nil), raw...)
	copy(want[off:], seed)
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatal("stream write changed bytes outside seed")
	}
	if err := Verify(bytes.NewReader(out.Bytes()), size, digest, nil); err != nil {
		t.Fatal(err)
	}
}
func TestGzipRejectsBeforeWrite(t *testing.T) {
	raw, _ := fixture()
	valid := compressedFixture(t, raw)
	corrupt := append([]byte(nil), valid...)
	corrupt[len(corrupt)-8] ^= 1
	badSlot := append([]byte(nil), raw...)
	badSlot[510] = 0
	cases := []struct {
		name     string
		data     []byte
		capacity int64
	}{
		{"checksum", corrupt, int64(len(raw))}, {"truncated", valid[:len(valid)-4], int64(len(raw))},
		{"capacity", valid, int64(len(raw) - 512)}, {"slot", compressedFixture(t, badSlot), int64(len(raw))},
		{"unaligned", compressedFixture(t, append(raw, 1)), int64(len(raw) + 512)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, err := InspectGzip(bytes.NewReader(tc.data), tc.capacity); err == nil {
				t.Fatal("bad image accepted")
			}
		})
	}
}
func TestGzipMetadataBackwardReads(t *testing.T) {
	raw, _ := fixture()
	r := &gzipReaderAt{source: bytes.NewReader(compressedFixture(t, raw))}
	defer r.close()
	for _, off := range []int64{1049088, 510, 1049600, 8192, 0} {
		b := make([]byte, 512)
		if _, e := r.ReadAt(b, off); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(b, raw[off:off+512]) {
			t.Fatal("wrong replay data")
		}
	}
}
func TestGzipConcatenatedMembers(t *testing.T) {
	raw, _ := fixture()
	mid := len(raw) / 2
	data := append(compressedFixture(t, raw[:mid]), compressedFixture(t, raw[mid:])...)
	n, _, hash, e := InspectGzip(bytes.NewReader(data), int64(len(raw)))
	if e != nil || n != int64(len(raw)) || hash != sha256.Sum256(raw) {
		t.Fatal("concatenated stream failed", e)
	}
}

func TestRealCompressedRelease(t *testing.T) {
	path := os.Getenv("VIOLET_TEST_GZIP")
	if path == "" {
		t.Skip("set VIOLET_TEST_GZIP for read-only validation of the 16 GiB release")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	size, offset, hash, err := InspectGzip(f, 16*1024*1024*1024)
	if err != nil || size != 16*1024*1024*1024 {
		t.Fatalf("release size=%d: %v", size, err)
	}
	t.Logf("size=%d seed offset=%d raw SHA256=%x", size, offset, hash)
}
