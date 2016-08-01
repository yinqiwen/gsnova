package helper

import (
	"errors"
	"log"
)

var ErrTLSIncomplete = errors.New("TLS header incomplete")
var ErrNoSNI = errors.New("No SNI in protocol")
var ErrTLSClientHello = errors.New("Invalid tls client hello")

func TLSParseSNI(data []byte) (string, error) {
	tlsHederLen := 5
	if len(data) < tlsHederLen {
		return "", ErrTLSIncomplete
	}
	if (int(data[0])&0x80) != 0 && data[2] == 1 {
		return "", ErrNoSNI
	}

	tlsContentType := int(data[0])
	if tlsContentType != 0x16 {
		log.Printf("Invaid content type:%d with %v", tlsContentType, ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	tlsMajorVer := int(data[1])
	tlsMinorVer := int(data[2])
	if tlsMajorVer < 3 {
		log.Printf("Invaid tls ver:%d with %v", tlsMajorVer, ErrNoSNI)
		return "", ErrNoSNI
	}

	tlsLen := (int(data[3]) << 8) + int(data[4]) + tlsHederLen
	if tlsLen > len(data) {
		return "", ErrTLSIncomplete
	}
	//log.Printf("####TLS %d %d", tlsLen, len(data))
	pos := tlsHederLen
	if pos+1 > len(data) {
		log.Printf("Less data 1 %v", ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	tlsHandshakeTypeClientHello := 0x01
	if int(data[pos]) != tlsHandshakeTypeClientHello {
		log.Printf("Not client hello type:%d with err:%v", data[pos], ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	/* Skip past fixed length records:
	   1	Handshake Type
	   3	Length
	   2	Version (again)
	   32	Random
	   to	Session ID Length
	*/
	pos += 38
	if pos+1 > len(data) {
		log.Printf("Less data 2 with err:%v", ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	nextLen := int(data[pos])
	pos = pos + 1 + nextLen

	if pos+2 > len(data) {
		log.Printf("Less data 3 with err:%v", ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	nextLen = (int(data[pos]) << 8) + int(data[pos+1])
	pos = pos + 2 + nextLen

	if pos+1 > len(data) {
		log.Printf("Less data 4 with err:%v", ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	nextLen = int(data[pos])
	pos = pos + 1 + nextLen

	if pos == len(data) && tlsMajorVer == 3 && tlsMinorVer == 0 {
		log.Printf("No sni in 3.0 %v", ErrNoSNI)
		return "", ErrNoSNI
	}

	if pos+2 > len(data) {
		log.Printf("Less data 5 with err:%v", ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	nextLen = (int(data[pos]) << 8) + int(data[pos+1])
	pos += 2
	if pos+nextLen > len(data) {
		log.Printf("Less data 6 with err:%v", ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	return parseExtension(data[pos:])
}

func parseExtension(data []byte) (string, error) {
	pos := 0
	for (pos + 4) <= len(data) {
		nextLen := (int(data[pos+2]) << 8) + int(data[pos+3])
		if int(data[pos]) == 0x00 && int(data[pos+1]) == 0x00 {
			if pos+4+nextLen > len(data) {
				log.Printf("Less data 7 with err:%v", ErrTLSClientHello)
				return "", ErrTLSClientHello
			}
			return parseServerNameExtension(data[pos+4:])
		}
		pos = pos + 4 + nextLen
	}
	if pos != len(data) {
		log.Printf("Less data 8 with err:%v", ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	return "", ErrNoSNI
}

func parseServerNameExtension(data []byte) (string, error) {
	pos := 2
	for pos+3 < len(data) {
		nextLen := (int(data[pos+1]) << 8) + int(data[pos+2])
		if pos+3+nextLen > len(data) {
			log.Printf("Less data 9 with err:%v", ErrTLSClientHello)
			return "", ErrTLSClientHello
		}

		if int(data[pos]) == 0x00 {
			name := make([]byte, nextLen)
			copy(name, data[pos+3:])
			return string(name), nil
		}
		pos = pos + 3 + nextLen
	}
	if pos != len(data) {
		log.Printf("Less data 10 with err:%v", ErrTLSClientHello)
		return "", ErrTLSClientHello
	}
	return "", ErrNoSNI

}
