// Package wire supplies bounded, little-endian payload primitives. Packet
// headers belong to version adapters and may use a different byte order.
package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

var ErrLimit = errors.New("wire length exceeds limit")
var ErrString = errors.New("invalid wire string")
var ErrTrailing = errors.New("unexpected trailing payload")

// Reader borrows an immutable payload. Errors are sticky; decoders must check
// Err or Finish before using decoded values. Bytes returns a borrowed slice.
type Reader struct {
	data []byte
	pos  int
	err  error
}

func NewReader(data []byte) *Reader { return &Reader{data: data} }
func (r *Reader) Err() error        { return r.err }
func (r *Reader) Remaining() int    { return len(r.data) - r.pos }
func (r *Reader) Finish() error {
	if r.err != nil {
		return r.err
	}
	if r.Remaining() != 0 {
		return ErrTrailing
	}
	return nil
}
func (r *Reader) Bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 {
		r.err = ErrLimit
		return nil
	}
	if n > r.Remaining() {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}
func (r *Reader) Uint8() uint8 {
	b := r.Bytes(1)
	if b == nil {
		return 0
	}
	return b[0]
}
func (r *Reader) Uint16() uint16 {
	b := r.Bytes(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}
func (r *Reader) Uint32() uint32 {
	b := r.Bytes(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}
func (r *Reader) Uint64() uint64 {
	b := r.Bytes(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}
func (r *Reader) Float32() float32 { return math.Float32frombits(r.Uint32()) }

// CString limits the bytes before the terminator; encoding validation belongs
// to the message/domain decoder, not the byte buffer.
func (r *Reader) CString(max int) string {
	if r.err != nil {
		return ""
	}
	if max < 0 {
		r.err = ErrLimit
		return ""
	}
	n := r.Remaining()
	if max < n {
		n = max + 1
	}
	i := bytes.IndexByte(r.data[r.pos:r.pos+n], 0)
	if i < 0 {
		if r.Remaining() > max {
			r.err = ErrLimit
		} else {
			r.err = io.ErrUnexpectedEOF
		}
		return ""
	}
	s := string(r.data[r.pos : r.pos+i])
	r.pos += i + 1
	return s
}
func (r *Reader) PackedGUID() uint64 {
	mask := r.Uint8()
	var guid uint64
	for i := uint(0); i < 8; i++ {
		if mask&(1<<i) != 0 {
			guid |= uint64(r.Uint8()) << (8 * i)
		}
	}
	return guid
}
