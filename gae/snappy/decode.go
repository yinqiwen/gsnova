package snappy

import (
	//"encoding/binary"
	"codec"
)

// ErrCorrupt reports that the input is invalid.
var ErrCorrupt = "snappy: corrupt input"

// DecodedLen returns the length of the decoded block.
func DecodedLen(src []byte) (int, bool, string) {
	v, _, success, err := decodedLen(src)
	return v, success, err
}

// decodedLen returns the length of the decoded block and the number of bytes
// that the length header occupied.
func decodedLen(src []byte) (blockLen, headerLen int, success bool, err string) {
	v, n := codec.Uvarint(src)
	if n == 0 {
		return 0, 0, false, ErrCorrupt
	}
	if uint64(int(v)) != v {
		return 0, 0, false,"snappy: decoded block is too large"
	}
	return int(v), n, true, ""
}

// Decode returns the decoded form of src. The returned slice may be a sub-
// slice of dst if dst was large enough to hold the entire decoded block.
// Otherwise, a newly allocated slice will be returned.
// It is valid to pass a nil dst.
func Decode(dst, src []byte) ([]byte, bool, string) {
	dLen, s, success, err := decodedLen(src)
	if !success {
		return nil, false, err
	}
	if len(dst) < dLen {
		dst = make([]byte, dLen)
	}

	var d, offset, length int
	for s < len(src) {
		switch src[s] & 0x03 {
		case tagLiteral:
			x := uint(src[s] >> 2)
			switch {
			case x < 60:
				s += 1
			case x == 60:
				s += 2
				if s > len(src) {
					return nil, false,ErrCorrupt
				}
				x = uint(src[s-1])
			case x == 61:
				s += 3
				if s > len(src) {
					return nil, false, ErrCorrupt
				}
				x = uint(src[s-2]) | uint(src[s-1])<<8
			case x == 62:
				s += 4
				if s > len(src) {
					return nil, false,ErrCorrupt
				}
				x = uint(src[s-3]) | uint(src[s-2])<<8 | uint(src[s-1])<<16
			case x == 63:
				s += 5
				if s > len(src) {
					return nil, false,ErrCorrupt
				}
				x = uint(src[s-4]) | uint(src[s-3])<<8 | uint(src[s-2])<<16 | uint(src[s-1])<<24
			}
			length = int(x + 1)
			if length <= 0 {
				return nil, false,"snappy: unsupported literal length"
			}
			if length > len(dst)-d || length > len(src)-s {
				return nil, false,ErrCorrupt
			}
			copy(dst[d:], src[s:s+length])
			d += length
			s += length
			continue

		case tagCopy1:
			s += 2
			if s > len(src) {
				return nil, false, ErrCorrupt
			}
			length = 4 + int(src[s-2])>>2&0x7
			offset = int(src[s-2])&0xe0<<3 | int(src[s-1])

		case tagCopy2:
			s += 3
			if s > len(src) {
				return nil, false,ErrCorrupt
			}
			length = 1 + int(src[s-3])>>2
			offset = int(src[s-2]) | int(src[s-1])<<8

		case tagCopy4:
			return nil,false, "snappy: unsupported COPY_4 tag"
		}

		end := d + length
		if offset > d || end > len(dst) {
			return nil, false,ErrCorrupt
		}
		for ; d < end; d++ {
			dst[d] = dst[d-offset]
		}
	}
	return dst, true,""
}
