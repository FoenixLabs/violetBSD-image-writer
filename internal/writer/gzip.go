package writer

import (
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
)

// InspectGzip validates the entire stream before any destination is opened.
// Counting decompressed bytes avoids the gzip trailer's 32-bit size wraparound.
// Metadata reads replay the stream as needed, keeping memory bounded and never
// expanding the image into a temporary file.
func InspectGzip(source io.ReadSeeker, capacity int64) (size, offset int64, digest [32]byte, err error) {
	if capacity <= 0 || capacity == int64(^uint64(0)>>1) {
		err = fmt.Errorf("invalid destination capacity")
		return
	}
	if _, err = source.Seek(0, io.SeekStart); err != nil {
		return
	}
	var gz *gzip.Reader
	gz, err = gzip.NewReader(source)
	if err != nil {
		return
	}
	h := sha256.New()
	size, err = io.CopyBuffer(h, io.LimitReader(gz, capacity+1), make([]byte, Chunk))
	gz.Close()
	if err != nil {
		return
	}
	if size > capacity || size == 0 || size%512 != 0 {
		err = fmt.Errorf("expanded image size does not fit the destination")
		return
	}
	copy(digest[:], h.Sum(nil))
	reader := &gzipReaderAt{source: source}
	defer reader.close()
	offset, err = SeedOffset(reader, size)
	return
}

type gzipReaderAt struct {
	source io.ReadSeeker
	gz     *gzip.Reader
	pos    int64
}

func (r *gzipReaderAt) close() {
	if r.gz != nil {
		r.gz.Close()
		r.gz = nil
	}
}
func (r *gzipReaderAt) ReadAt(b []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, fmt.Errorf("negative gzip offset")
	}
	if r.gz == nil || offset < r.pos {
		r.close()
		if _, err := r.source.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
		gz, err := gzip.NewReader(r.source)
		if err != nil {
			return 0, err
		}
		r.gz = gz
		r.pos = 0
	}
	skipped, err := io.CopyN(io.Discard, r.gz, offset-r.pos)
	r.pos += skipped
	if err != nil {
		return 0, err
	}
	n, err := io.ReadFull(r.gz, b)
	r.pos += int64(n)
	return n, err
}
