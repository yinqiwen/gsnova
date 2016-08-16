package helper

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/netx"
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

func Long2IPv4(i uint64) string {
	return fmt.Sprintf("%d.%d.%d.%d", (i>>24)&0xFF, (i>>16)&0xFF, (i>>8)&0xFF, i&0xFF)
}

func IPv42Int(ip string) (int64, error) {
	addrArray := strings.Split(ip, ".")
	var num int64
	num = 0
	for i := 0; i < len(addrArray); i++ {
		power := 3 - i
		if v, err := strconv.Atoi(addrArray[i]); nil != err {
			return -1, err
		} else {
			num += (int64(v) % 256 * int64(math.Pow(float64(256), float64(power))))
		}
	}
	return num, nil
}

func IsPrivateIP(ip string) bool {
	if strings.EqualFold(ip, "localhost") {
		return true
	}
	value, err := IPv42Int(ip)
	if nil != err {
		return false
	}
	if strings.HasPrefix(ip, "127.0") {
		return true
	}
	if (value >= 0x0A000000 && value <= 0x0AFFFFFF) || (value >= 0xAC100000 && value <= 0xAC1FFFFF) || (value >= 0xC0A80000 && value <= 0xC0A8FFFF) {
		return true
	}
	return false
}

func HTTPProxyConn(proxyURL string, addr string, timeout time.Duration) (net.Conn, error) {
	u, err := url.Parse(proxyURL)
	if nil != err {
		return nil, err
	}
	c, err := netx.DialTimeout("tcp", u.Host, timeout)
	if err != nil {
		return nil, err
	}
	connReq, _ := http.NewRequest("Connect", addr, nil)
	err = connReq.Write(c)
	if err != nil {
		return nil, err
	}
	connRes, err := http.ReadResponse(bufio.NewReader(c), connReq)
	if err != nil {
		return nil, err
	}
	if nil != connRes.Body {
		connRes.Body.Close()
	}
	if connRes.StatusCode >= 300 {
		return nil, fmt.Errorf("Invalid Connect response:%d", connRes.StatusCode)
	}
	return c, nil
}
