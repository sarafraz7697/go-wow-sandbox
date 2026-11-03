package frame

// DOMAIN_WORLD_SPLIT_PATCH: moved from internal/protocol/world/tbc/stream.go.

import (
	"io"

	"wowserver/internal/protocol/world"
	"wowserver/internal/protocol/world/tbc/crypt"
)

// Reader is single-owner and reads exactly one packet per call. It deliberately
// does not read ahead: the owner can enable cryptography after AUTH_SESSION
// before reading an encrypted packet already queued in the same TCP stream.
// Any read/protocol error is terminal: discard this reader and the connection.
type Reader struct {
	source io.Reader
	cipher *crypt.Cipher
	err    error
}

func NewReader(source io.Reader) *Reader { return &Reader{source: source} }

func (r *Reader) EnableEncryption(key [40]byte) error {
	if r.err != nil {
		return r.err
	}
	if r.cipher != nil {
		return crypt.ErrCryptState
	}
	c, err := crypt.NewCipher(key)
	if err != nil {
		return err
	}
	r.cipher = c
	return nil
}

func (r *Reader) ReadPacket() (world.Packet, error) {
	if r.err != nil {
		return world.Packet{}, r.err
	}
	p, err := r.readPacket()
	if err != nil {
		r.err = err
	}
	return p, err
}

func (r *Reader) readPacket() (world.Packet, error) {
	var header [ClientHeaderSize]byte
	if _, err := io.ReadFull(r.source, header[:]); err != nil {
		return world.Packet{}, err
	}
	if r.cipher != nil {
		r.cipher.Decrypt(header[:])
	}
	op, size, err := DecodeClientHeader(header[:])
	if err != nil {
		return world.Packet{}, err
	}
	p := world.Packet{Opcode: op, Payload: make([]byte, size)}
	if _, err = io.ReadFull(r.source, p.Payload); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return world.Packet{}, err
	}
	return p, nil
}

// Writer is single-owner. A failed/partial write poisons it because header
// cipher state has already advanced; retrying that packet is unsafe.
type Writer struct {
	destination io.Writer
	cipher      *crypt.Cipher
	err         error
}

func NewWriter(destination io.Writer) *Writer { return &Writer{destination: destination} }

func (w *Writer) EnableEncryption(key [40]byte) error {
	if w.err != nil {
		return w.err
	}
	if w.cipher != nil {
		return crypt.ErrCryptState
	}
	c, err := crypt.NewCipher(key)
	if err != nil {
		return err
	}
	w.cipher = c
	return nil
}

func (w *Writer) WritePacket(packet world.Packet) error {
	if w.err != nil {
		return w.err
	}
	frame, err := EncodeServer(packet)
	if err != nil {
		return err
	} // no state consumed
	if w.cipher != nil {
		w.cipher.Encrypt(frame[:ServerHeaderSize])
	}
	for len(frame) > 0 {
		n, err := w.destination.Write(frame)
		if n < 0 || n > len(frame) {
			w.err = io.ErrShortWrite
			return w.err
		}
		frame = frame[n:]
		if err != nil {
			w.err = err
			return err
		}
		if n == 0 {
			w.err = io.ErrShortWrite
			return w.err
		}
	}
	return nil
}
