// Package auth handles the legacy login envelope. World framing is separate.
package auth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var ErrMalformed = errors.New("malformed login challenge")
var ErrCommand = errors.New("unsupported command in challenge state")

type Challenge struct {
	Command byte
	Version [3]byte
	Build   uint16
	Account string
}

// ReadChallenge reads exactly one AUTH_LOGON_CHALLENGE, leaving subsequent
// bytes in the stream. Its body is 30 fixed bytes plus the account name.
// This bootstrap format is legacy-specific, not a universal WoW envelope.
func ReadChallenge(r io.Reader, maxBody int) (Challenge, error) {
	return ReadChallengeTrace(r, maxBody, func(string, ...any) {})
}

// ReadChallengeTrace emits metadata only, never account or packet payloads.
func ReadChallengeTrace(r io.Reader, maxBody int, trace func(string, ...any)) (Challenge, error) {
	var result Challenge
	var header [4]byte
	if _, err := io.ReadFull(r, header[:1]); err != nil {
		return result, err
	}
	trace("challenge command received", "opcode", header[0])
	if header[0] != 0 && header[0] != 2 {
		return result, ErrCommand
	}
	result.Command = header[0]
	if _, err := io.ReadFull(r, header[1:]); err != nil {
		return result, err
	}
	n := int(binary.LittleEndian.Uint16(header[2:]))
	trace("challenge header received", "body_bytes", n, "protocol", header[1])
	if n < 31 || n > 285 || n > maxBody {
		return result, ErrMalformed
	}
	body := make([]byte, n)
	if received, err := io.ReadFull(r, body); err != nil {
		trace("challenge body incomplete", "received_bytes", received, "expected_bytes", n)
		return result, err
	}
	trace("challenge body received", "body_bytes", n, "game_tag", fmt.Sprintf("%x", body[:4]))
	return DecodeChallenge(header[0], body)
}

// DecodeChallenge decodes a complete legacy challenge body without I/O.
func DecodeChallenge(command byte, body []byte) (Challenge, error) {
	result := Challenge{Command: command}
	if command != 0 && command != 2 {
		return result, ErrCommand
	}
	n := len(body)
	if n < 31 || n > 285 {
		return result, ErrMalformed
	}
	if string(body[:4]) != "WoW\x00" || int(body[29]) != n-30 {
		return result, ErrMalformed
	}
	copy(result.Version[:], body[4:7])
	result.Build = binary.LittleEndian.Uint16(body[7:9])
	result.Account = string(body[30:])
	return result, nil
}

// WriteAll handles short writes without silently discarding bytes.
func WriteAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n < 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
