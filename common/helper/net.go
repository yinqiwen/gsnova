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

func TLSReplaceSNI(data []byte, sni string) ([]byte, string, error) {
	name, offset, err := tlsParseSNI(data)
	if nil == err {
		newData := make([]byte, offset)
		copy(newData, data[0:offset])
		//newData := data[0:offset]
		tailData := data[offset+2+len(name):]
		sniData := make([]byte, 2+len(sni))
		sniData[0] = uint8(uint16(len(sni)) >> 8)
		sniData[1] = uint8(uint16(len(sni)) & 0xFF)
		copy(sniData[2:], []byte(sni))
		newData = append(newData, sniData...)
		newData = append(newData, tailData...)
		if name == sni {
			return data, name, nil
		}
		return newData, name, nil
	}
	return data, name, err
}

func TLSParseSNI(data []byte) (string, error) {
	name, _, err := tlsParseSNI(data)
	return name, err
}

func tlsParseSNI(data []byte) (string, int, error) {
	tlsHederLen := 5
	if len(data) < tlsHederLen {
		return "", 0, ErrTLSIncomplete
	}
	if (int(data[0])&0x80) != 0 && data[2] == 1 {
		return "", 0, ErrNoSNI
	}

	tlsContentType := int(data[0])
	if tlsContentType != 0x16 {
		log.Printf("Invaid content type:%d with %v", tlsContentType, ErrTLSClientHello)
		return "", 0, ErrTLSClientHello
	}
	tlsMajorVer := int(data[1])
	tlsMinorVer := int(data[2])
	if tlsMajorVer < 3 {
		log.Printf("Invaid tls ver:%d with %v", tlsMajorVer, ErrNoSNI)
		return "", 0, ErrNoSNI
	}

	tlsLen := (int(data[3]) << 8) + int(data[4]) + tlsHederLen
	if tlsLen > len(data) {
		return "", 0, ErrTLSIncomplete
	}
	//log.Printf("####TLS %d %d", tlsLen, len(data))
	pos := tlsHederLen
	if pos+1 > len(data) {
		log.Printf("Less data 1 %v", ErrTLSClientHello)
		return "", 0, ErrTLSClientHello
	}
	tlsHandshakeTypeClientHello := 0x01
	if int(data[pos]) != tlsHandshakeTypeClientHello {
		log.Printf("Not client hello type:%d with err:%v", data[pos], ErrTLSClientHello)
		return "", 0, ErrTLSClientHello
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
		return "", 0, ErrTLSClientHello
	}
	nextLen := int(data[pos])
	pos = pos + 1 + nextLen

	if pos+2 > len(data) {
		log.Printf("Less data 3 with err:%v", ErrTLSClientHello)
		return "", 0, ErrTLSClientHello
	}
	nextLen = (int(data[pos]) << 8) + int(data[pos+1])
	pos = pos + 2 + nextLen

	if pos+1 > len(data) {
		log.Printf("Less data 4 with err:%v", ErrTLSClientHello)
		return "", 0, ErrTLSClientHello
	}
	nextLen = int(data[pos])
	pos = pos + 1 + nextLen

	if pos == len(data) && tlsMajorVer == 3 && tlsMinorVer == 0 {
		log.Printf("No sni in 3.0 %v", ErrNoSNI)
		return "", 0, ErrNoSNI
	}

	if pos+2 > len(data) {
		log.Printf("Less data 5 with err:%v", ErrTLSClientHello)
		return "", 0, ErrTLSClientHello
	}
	nextLen = (int(data[pos]) << 8) + int(data[pos+1])
	pos += 2
	if pos+nextLen > len(data) {
		log.Printf("Less data 6 with err:%v", ErrTLSClientHello)
		return "", 0, ErrTLSClientHello
	}
	return parseExtension(data[pos:], pos)
}

func parseExtension(data []byte, offset int) (string, int, error) {
	pos := 0
	for (pos + 4) <= len(data) {
		nextLen := (int(data[pos+2]) << 8) + int(data[pos+3])
		if int(data[pos]) == 0x00 && int(data[pos+1]) == 0x00 {
			if pos+4+nextLen > len(data) {
				log.Printf("Less data 7 with err:%v", ErrTLSClientHello)
				return "", 0, ErrTLSClientHello
			}
			offset = offset + pos + 4
			return parseServerNameExtension(data[pos+4:], offset)
		}
		pos = pos + 4 + nextLen
	}
	if pos != len(data) {
		log.Printf("Less data 8 with err:%v", ErrTLSClientHello)
		return "", 0, ErrTLSClientHello
	}
	return "", 0, ErrNoSNI
}

func parseServerNameExtension(data []byte, offset int) (string, int, error) {
	pos := 2
	for pos+3 < len(data) {
		nextLen := (int(data[pos+1]) << 8) + int(data[pos+2])
		if pos+3+nextLen > len(data) {
			log.Printf("Less data 9 with err:%v", ErrTLSClientHello)
			return "", 0, ErrTLSClientHello
		}

		if int(data[pos]) == 0x00 {
			offset = offset + pos + 1
			name := make([]byte, nextLen)
			copy(name, data[pos+3:])
			return string(name), offset, nil
		}
		pos = pos + 3 + nextLen
	}
	if pos != len(data) {
		log.Printf("Less data 10 with err:%v", ErrTLSClientHello)
		return "", 0, ErrTLSClientHello
	}
	return "", 0, ErrNoSNI
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

var localIPv4 []string

func GetLocalIPv4() []string {
	if len(localIPv4) > 0 {
		return localIPv4
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("[ERROR]Failed to get local ip:%v", err)
		return localIPv4
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				localIPv4 = append(localIPv4, ipnet.IP.String())
			}
		}
	}
	return localIPv4
}
