// Package dbc reads bounded WDBC files. Field widths belong to a versioned
// schema: field count does not imply record size (some fields are single bytes).
package dbc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const maxFileSize = 128 << 20

type File struct {
	rows, size       int
	records, strings []byte
}

func Open(path string, fields, recordSize uint32) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 20 || info.Size() > maxFileSize {
		return nil, fmt.Errorf("%s: invalid DBC file size", path)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	d, err := Decode(b, fields, recordSize)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

// Decode copies input; callers cannot mutate a published table through b.
func Decode(b []byte, fields, recordSize uint32) (*File, error) {
	bad := func() (*File, error) { return nil, fmt.Errorf("invalid WDBC header, layout or string block") }
	if len(b) < 20 || len(b) > maxFileSize || string(b[:4]) != "WDBC" || recordSize == 0 {
		return bad()
	}
	rows := binary.LittleEndian.Uint32(b[4:])
	fc := binary.LittleEndian.Uint32(b[8:])
	rs := binary.LittleEndian.Uint32(b[12:])
	ss := binary.LittleEndian.Uint32(b[16:])
	n := uint64(rows) * uint64(rs)
	if fc != fields || rs != recordSize || ss == 0 || 20+n+uint64(ss) != uint64(len(b)) {
		return bad()
	}
	start := 20 + int(n)
	if b[start] != 0 || b[len(b)-1] != 0 {
		return bad()
	}
	owned := bytes.Clone(b[20:])
	return &File{int(rows), int(rs), owned[:int(n)], owned[int(n):]}, nil
}
func (f *File) Len() int { return f.rows }
func (f *File) Uint32(row, offset int) (uint32, error) {
	if row < 0 || row >= f.rows || offset < 0 || offset > f.size-4 {
		return 0, fmt.Errorf("DBC uint32 outside record")
	}
	return binary.LittleEndian.Uint32(f.records[row*f.size+offset:]), nil
}
func (f *File) Byte(row, offset int) (byte, error) {
	if row < 0 || row >= f.rows || offset < 0 || offset >= f.size {
		return 0, fmt.Errorf("DBC byte outside record")
	}
	return f.records[row*f.size+offset], nil
}
func (f *File) String(row, offset int) (string, error) {
	index, err := f.Uint32(row, offset)
	if err != nil {
		return "", err
	}
	if uint64(index) >= uint64(len(f.strings)) {
		return "", fmt.Errorf("DBC string offset outside block")
	}
	b := f.strings[index:]
	end := bytes.IndexByte(b, 0)
	if end < 0 {
		return "", fmt.Errorf("unterminated DBC string")
	}
	return string(b[:end]), nil
}
