package mux

import "errors"

const (
	DefaultMuxCipherMethod         = "chacha20poly1305"
	DefaultMuxInitialCipherCounter = uint64(47816489)
	AuthOK                         = 1
)

var (
	ErrToolargeMessage = errors.New("too large message length")
	ErrAuthFailed      = errors.New("auth failed")
	ErrDataReadMissing = errors.New("auth failed")
)
