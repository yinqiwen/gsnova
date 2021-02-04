package mux

import "errors"

const (
	DefaultMuxCipherMethod         = "chacha20poly1305"
	DefaultMuxInitialCipherCounter = uint64(47816489)
	AuthOK                         = 1

	//GZipCompressor   = "gzip"

	HTTPMuxSessionIDHeader    = "X-Session-ID"
	HTTPMuxSessionACKIDHeader = "X-Session-ACK-ID"
	HTTPMuxPullPeriodHeader   = "X-PullPeriod"
)

var (
	ErrToolargeMessage = errors.New("too large message length")
	ErrAuthFailed      = errors.New("auth failed")
	ErrDataReadMissing = errors.New("auth failed")
)
