// Package writer personalizes the reserved entropy file without allocating UFS blocks.
package writer

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1" // Required by the NetBSD rndsave_t file format, not used as a RNG.
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const SeedSize = 536
const Chunk = 4 * 1024 * 1024

var be = binary.BigEndian

func Seed() ([]byte, error) {
	b := make([]byte, SeedSize)
	be.PutUint32(b, 256)
	if _, err := io.ReadFull(rand.Reader, b[4:516]); err != nil {
		return nil, err
	}
	sum := sha1.Sum(b[:516])
	copy(b[516:], sum[:])
	return b, nil
}

// SeedOffset accepts only the current 512-byte-sector, big-endian UFS1 release layout.
// Every read is bounded; unsupported geometry, links and indirect blocks fail closed.
func SeedOffset(r io.ReaderAt, size int64) (int64, error) {
	read := func(off, n int64) ([]byte, error) {
		if off < 0 || n < 0 || off > size || n > size-off {
			return nil, errors.New("filesystem extent outside image")
		}
		b := make([]byte, n)
		_, err := r.ReadAt(b, off)
		return b, err
	}
	mbr, err := read(510, 2)
	if err != nil {
		return 0, err
	}
	if !bytes.Equal(mbr, []byte{0x55, 0xaa}) {
		return 0, errors.New("invalid MBR")
	}
	label, err := read(2049*512, 512)
	if err != nil {
		return 0, err
	}
	u := func(b []byte, off int) int64 { return int64(be.Uint32(b[off:])) }
	if u(label, 0) != 0x82564557 || u(label, 132) != 0x82564557 {
		return 0, errors.New("invalid big-endian disklabel")
	}
	root := u(label, 152) * 512
	end := root + u(label, 148)*512
	if root < 2050*512 || end > size || end <= root {
		return 0, errors.New("invalid root filesystem bounds")
	}
	fsread := func(off, n int64) ([]byte, error) {
		if off < root || off > end || n > end-off {
			return nil, errors.New("extent outside root filesystem")
		}
		return read(off, n)
	}
	sb, err := fsread(root+8192, 8192)
	if err != nil {
		return 0, err
	}
	bs, frag, ipg, fpg := u(sb, 48), u(sb, 52), u(sb, 184), u(sb, 188)
	if u(sb, 1372) != 0x11954 || bs != 8192 || frag != 1024 || ipg == 0 || fpg == 0 {
		return 0, errors.New("unsupported UFS1 geometry")
	}
	inode := func(num int64) ([]byte, error) {
		cg, idx := num/ipg, num%ipg
		if cg >= u(sb, 44) {
			return nil, errors.New("invalid inode number")
		}
		start := cg*fpg + u(sb, 24)*(cg & ^u(sb, 28))
		return fsread(root+(start+u(sb, 16))*frag+idx*128, 128)
	}
	node, err := inode(2)
	if err != nil {
		return 0, err
	}
	for _, name := range []string{"var", "db", "entropy-file"} {
		n := be.Uint64(node[8:])
		if be.Uint16(node)&0xf000 != 0x4000 || n > uint64(12*bs) {
			return 0, errors.New("unsupported directory")
		}
		var data []byte
		for off := int64(0); off < int64(n); off += bs {
			block := u(node, 40+4*int(off/bs))
			if block == 0 {
				return 0, errors.New("sparse directory")
			}
			b, e := fsread(root+block*frag, min(bs, int64(n)-off))
			if e != nil {
				return 0, e
			}
			data = append(data, b...)
		}
		var found []int64
		for off := 0; off < len(data); {
			if len(data)-off < 8 {
				return 0, errors.New("truncated directory")
			}
			num := u(data, off)
			reclen := int(be.Uint16(data[off+4:]))
			namelen := int(data[off+7])
			if reclen < 8 || reclen%4 != 0 || reclen > len(data)-off || namelen > reclen-8 {
				return 0, errors.New("invalid directory record")
			}
			if num != 0 && string(data[off+8:off+8+namelen]) == name {
				found = append(found, num)
			}
			off += reclen
		}
		if len(found) != 1 {
			return 0, errors.New("image lacks a unique entropy slot; use an updated violetBSD release image")
		}
		node, err = inode(found[0])
		if err != nil {
			return 0, err
		}
	}
	if be.Uint16(node) != 0100600 || be.Uint16(node[2:]) != 1 || u(node, 112) != 0 || u(node, 116) != 0 || be.Uint64(node[8:]) != SeedSize || u(node, 40) == 0 || !bytes.Equal(node[44:100], make([]byte, 56)) {
		return 0, errors.New("unsafe entropy-file inode")
	}
	offset := root + u(node, 40)*frag
	seed, err := fsread(offset, SeedSize)
	if err != nil {
		return 0, err
	}
	sum := sha1.Sum(seed[:516])
	if !bytes.Equal(sum[:], seed[516:]) {
		return 0, errors.New("invalid initial seed checksum")
	}
	return offset, nil
}

// Copy overlays the secret only in memory. The input image remains unchanged.
func Copy(dst io.Writer, src io.Reader, size, offset int64, seed []byte, progress func(int64)) ([32]byte, error) {
	var zero [32]byte
	if len(seed) != SeedSize || offset < 0 || offset > size-SeedSize || size%512 != 0 {
		return zero, errors.New("invalid image extent")
	}
	digest := sha256.New()
	buf := make([]byte, Chunk)
	for pos := int64(0); pos < size; {
		n := int(min(int64(len(buf)), size-pos))
		block := buf[:n]
		if _, err := io.ReadFull(src, block); err != nil {
			return zero, fmt.Errorf("image read: %w", err)
		}
		a, b := max(pos, offset), min(pos+int64(n), offset+SeedSize)
		if a < b {
			copy(block[a-pos:b-pos], seed[a-offset:b-offset])
		}
		for remaining := block; len(remaining) > 0; {
			written, err := dst.Write(remaining)
			if err != nil {
				return zero, err
			}
			if written <= 0 || written > len(remaining) {
				return zero, io.ErrShortWrite
			}
			remaining = remaining[written:]
		}
		digest.Write(block)
		pos += int64(n)
		if progress != nil {
			progress(pos)
		}
	}
	copy(zero[:], digest.Sum(nil))
	return zero, nil
}

func Verify(r io.Reader, size int64, expected [32]byte, progress func(int64)) error {
	h := sha256.New()
	buf := make([]byte, Chunk)
	for pos := int64(0); pos < size; {
		n := min(int64(len(buf)), size-pos)
		b := buf[:n]
		if _, err := io.ReadFull(r, b); err != nil {
			return fmt.Errorf("readback: %w", err)
		}
		h.Write(b)
		pos += n
		if progress != nil {
			progress(pos)
		}
	}
	if !bytes.Equal(h.Sum(nil), expected[:]) {
		return errors.New("destination readback mismatch")
	}
	return nil
}
