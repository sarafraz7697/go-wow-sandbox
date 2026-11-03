package tbc

import "wowserver/internal/protocol/world/tbc/codec"

var authResponse = codec.AuthResponse
var errWorldAuth = codec.ErrWorldAuth
var errWorldPacket = codec.ErrWorldPacket

const authOK = codec.AuthOK
const authFailed = codec.AuthFailed
const authAlreadyOnline = codec.AuthAlreadyOnline
const authVersionMismatch = codec.AuthVersionMismatch
