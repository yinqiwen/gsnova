package helper

import (
	"errors"

	"github.com/yinqiwen/gsnova/common/logger"
)

var ErrTLSIncomplete = errors.New("TLS header incomplete")
var ErrNoSNI = errors.New("No SNI in protocol")
var ErrTLSClientHello = errors.New("Invalid tls client hello")

func PeekTLSServerName(reader PeekReader) (string, error) {
	tlsHederLen := 5
	hbuf, err := reader.Peek(tlsHederLen)
	if nil != err {
		return "", err
	}
	if hbuf[0] != 0x16 { // recordTypeHandshake
		//log.Printf("####1err:%d", hbuf[0])
		return "", ErrTLSClientHello
	}
	tlsMajorVer := int(hbuf[1])
	tlsMinorVer := int(hbuf[2])
	if tlsMajorVer < 3 {
		logger.Error("Invaid tls ver:%d.%v with %v", tlsMajorVer, tlsMinorVer, ErrNoSNI)
		return "", ErrNoSNI
	}
	restLen := (int(hbuf[3]) << 8) + int(hbuf[4])
	restBuf, err := reader.Peek(tlsHederLen + restLen)
	if nil != err {
		return "", err
	}
	restBuf = restBuf[tlsHederLen:]

	tlsHandshakeTypeClientHello := 0x01
	if int(restBuf[0]) != tlsHandshakeTypeClientHello {
		return "", ErrTLSClientHello
	}
	/* Skip past fixed length records:
	   1	Handshake Type
	   3	Length
	   2	Version (again)
	   32	Random
	   to	Session ID Length
	*/
	if len(restBuf) < 42 {
		return "", ErrNoSNI
	}
	restBuf = restBuf[38:]
	sessionIDLen := int(restBuf[0])
	restBuf = restBuf[1+sessionIDLen:]
	if len(restBuf) < 2 {
		return "", ErrTLSClientHello
	}
	// cipherSuiteLen is the number of bytes of cipher suite numbers. Since
	// they are uint16s, the number must be even.
	cipherSuiteLen := int(restBuf[0])<<8 | int(restBuf[1])
	if cipherSuiteLen%2 == 1 || len(restBuf) < 2+cipherSuiteLen {
		return "", ErrTLSClientHello
	}
	restBuf = restBuf[2+cipherSuiteLen:]

	compressionMethodsLen := int(restBuf[0])
	if len(restBuf) < 1+compressionMethodsLen {
		return "", ErrTLSClientHello
	}
	restBuf = restBuf[1+compressionMethodsLen:]

	if len(restBuf) < 2 {
		return "", ErrNoSNI
	}
	extensionsLength := int(restBuf[0])<<8 | int(restBuf[1])
	restBuf = restBuf[2:]
	if extensionsLength != len(restBuf) {
		return "", ErrTLSClientHello
	}
	for len(restBuf) != 0 {
		if len(restBuf) < 4 {
			return "", ErrTLSClientHello
		}
		extension := uint16(restBuf[0])<<8 | uint16(restBuf[1])
		length := int(restBuf[2])<<8 | int(restBuf[3])
		restBuf = restBuf[4:]
		if len(restBuf) < length {
			return "", ErrTLSClientHello
		}

		switch extension {
		case 0x00:
			if length < 2 {
				return "", ErrTLSClientHello
			}
			numNames := int(restBuf[0])<<8 | int(restBuf[1])
			d := restBuf[2:]
			for i := 0; i < numNames; i++ {
				if len(d) < 3 {
					return "", ErrTLSClientHello
				}
				nameType := d[0]
				nameLen := int(d[1])<<8 | int(d[2])
				d = d[3:]
				if len(d) < nameLen {
					return "", ErrTLSClientHello
				}
				if nameType == 0 {
					serverName := string(d[0:nameLen])
					return serverName, nil
				}
				d = d[nameLen:]
			}
		}
		restBuf = restBuf[length:]
	}
	return "", ErrTLSClientHello
}
