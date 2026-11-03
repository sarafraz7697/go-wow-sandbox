package wire

import (
	"encoding/binary"
	"math"
	"strings"
)

// Writer owns its buffer and never grows beyond max. Result transfers the
// completed bytes to the caller; do not reuse the writer after that transfer.
type Writer struct {
	data []byte
	max  int
	err  error
}

func NewWriter(max int) *Writer {
	w := &Writer{max: max}
	if max < 0 {
		w.err = ErrLimit
	}
	return w
}
func (w *Writer) reserve(n int) []byte {
	if w.err != nil {
		return nil
	}
	if n < 0 || n > w.max-len(w.data) {
		w.err = ErrLimit
		return nil
	}
	p := len(w.data)
	w.data = append(w.data, make([]byte, n)...)
	return w.data[p:]
}
func (w *Writer) Result() ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}
	return w.data, nil
}
func (w *Writer) Bytes(b []byte) { copy(w.reserve(len(b)), b) }
func (w *Writer) Uint8(v uint8) {
	if b := w.reserve(1); b != nil {
		b[0] = v
	}
}
func (w *Writer) Uint16(v uint16) {
	if b := w.reserve(2); b != nil {
		binary.LittleEndian.PutUint16(b, v)
	}
}
func (w *Writer) Uint32(v uint32) {
	if b := w.reserve(4); b != nil {
		binary.LittleEndian.PutUint32(b, v)
	}
}
func (w *Writer) Uint64(v uint64) {
	if b := w.reserve(8); b != nil {
		binary.LittleEndian.PutUint64(b, v)
	}
}
func (w *Writer) Float32(v float32) { w.Uint32(math.Float32bits(v)) }
func (w *Writer) CString(s string, max int) {
	if w.err != nil {
		return
	}
	if max < 0 || len(s) > max {
		w.err = ErrLimit
		return
	}
	if strings.IndexByte(s, 0) >= 0 {
		w.err = ErrString
		return
	}
	if len(s) >= w.max-len(w.data) {
		w.err = ErrLimit
		return
	}
	b := w.reserve(len(s) + 1)
	copy(b, s)
}
func (w *Writer) PackedGUID(guid uint64) {
	var b [9]byte
	n := 1
	for i := uint(0); i < 8; i++ {
		v := byte(guid >> (8 * i))
		if v != 0 {
			b[0] |= 1 << i
			b[n] = v
			n++
		}
	}
	w.Bytes(b[:n])
}
