package tbc

// DOMAIN_WORLD_SPLIT_PATCH: frame and crypt now live in their own subpackages
// (frame/, crypt/). These temporary aliases keep the existing call sites and
// package tests compiling without behavior changes. Remove them in a later
// cleanup slice once callers use the subpackages directly. Tests that build
// headerCipher literals must switch to crypt.Cipher / Encrypt / Decrypt.

import (
	"wowserver/internal/protocol/world/tbc/crypt"
	"wowserver/internal/protocol/world/tbc/frame"
)

type Reader = frame.Reader
type Writer = frame.Writer

var (
	NewReader          = frame.NewReader
	NewWriter          = frame.NewWriter
	DecodeClientHeader = frame.DecodeClientHeader
	EncodeServer       = frame.EncodeServer
	ErrHeader          = frame.ErrHeader
	ErrSize            = frame.ErrSize
	ErrOpcode          = frame.ErrOpcode
	ErrCryptState      = crypt.ErrCryptState
)

const (
	ClientHeaderSize = frame.ClientHeaderSize
	ServerHeaderSize = frame.ServerHeaderSize
	MaxClientSize    = frame.MaxClientSize
	MaxServerPayload = frame.MaxServerPayload
)

// deriveKey remains only so tests of the moved implementation keep compiling;
// prefer crypt.DeriveKey.
func deriveKey(session [40]byte) [20]byte { return crypt.DeriveKey(session) }

// headerCipher aliases the exported cipher type; unexported fields are not
// accessible through the alias outside package crypt.
type headerCipher = crypt.Cipher
