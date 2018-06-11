package helper

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/common/protector"
)

var ErrWriteTimeout = errors.New("write timeout")
var ErrReadTimeout = errors.New("read timeout")
var ErrConnReset = errors.New("Conn reset")

// func TLSReplaceSNI(data []byte, sni string) ([]byte, string, error) {
// 	name, offset, err := tlsParseSNI(data)
// 	if nil == err {
// 		newData := make([]byte, offset)
// 		copy(newData, data[0:offset])
// 		//newData := data[0:offset]
// 		tailData := data[offset+2+len(name):]
// 		sniData := make([]byte, 2+len(sni))
// 		sniData[0] = uint8(uint16(len(sni)) >> 8)
// 		sniData[1] = uint8(uint16(len(sni)) & 0xFF)
// 		copy(sniData[2:], []byte(sni))
// 		newData = append(newData, sniData...)
// 		newData = append(newData, tailData...)
// 		if name == sni {
// 			return data, name, nil
// 		}
// 		return newData, name, nil
// 	}
// 	return data, name, err
// }

// func TLSParseSNI(data []byte) (string, error) {
// 	name, _, err := tlsParseSNI(data)
// 	return name, err
// }

// func tlsParseSNI(data []byte) (string, int, error) {
// 	tlsHederLen := 5
// 	if len(data) < tlsHederLen {
// 		return "", 0, ErrTLSIncomplete
// 	}
// 	if (int(data[0])&0x80) != 0 && data[2] == 1 {
// 		return "", 0, ErrNoSNI
// 	}

// 	tlsContentType := int(data[0])
// 	if tlsContentType != 0x16 {
// 		log.Printf("Invaid content type:%d with %v", tlsContentType, ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	tlsMajorVer := int(data[1])
// 	tlsMinorVer := int(data[2])
// 	if tlsMajorVer < 3 {
// 		log.Printf("Invaid tls ver:%d with %v", tlsMajorVer, ErrNoSNI)
// 		return "", 0, ErrNoSNI
// 	}

// 	tlsLen := (int(data[3]) << 8) + int(data[4]) + tlsHederLen
// 	if tlsLen > len(data) {
// 		return "", 0, ErrTLSIncomplete
// 	}
// 	//log.Printf("####TLS %d %d", tlsLen, len(data))
// 	pos := tlsHederLen
// 	if pos+1 > len(data) {
// 		log.Printf("Less data 1 %v", ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	tlsHandshakeTypeClientHello := 0x01
// 	if int(data[pos]) != tlsHandshakeTypeClientHello {
// 		log.Printf("Not client hello type:%d with err:%v", data[pos], ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	/* Skip past fixed length records:
// 	   1	Handshake Type
// 	   3	Length
// 	   2	Version (again)
// 	   32	Random
// 	   to	Session ID Length
// 	*/
// 	pos += 38
// 	if pos+1 > len(data) {
// 		log.Printf("Less data 2 with err:%v", ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	nextLen := int(data[pos])
// 	pos = pos + 1 + nextLen

// 	if pos+2 > len(data) {
// 		log.Printf("Less data 3 with err:%v", ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	nextLen = (int(data[pos]) << 8) + int(data[pos+1])
// 	pos = pos + 2 + nextLen

// 	if pos+1 > len(data) {
// 		log.Printf("Less data 4 with err:%v", ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	nextLen = int(data[pos])
// 	pos = pos + 1 + nextLen

// 	if pos == len(data) && tlsMajorVer == 3 && tlsMinorVer == 0 {
// 		log.Printf("No sni in 3.0 %v", ErrNoSNI)
// 		return "", 0, ErrNoSNI
// 	}

// 	if pos+2 > len(data) {
// 		log.Printf("Less data 5 with err:%v", ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	nextLen = (int(data[pos]) << 8) + int(data[pos+1])
// 	pos += 2
// 	if pos+nextLen > len(data) {
// 		log.Printf("Less data 6 with err:%v", ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	return parseExtension(data[pos:], pos)
// }

// func parseExtension(data []byte, offset int) (string, int, error) {
// 	pos := 0
// 	for (pos + 4) <= len(data) {
// 		nextLen := (int(data[pos+2]) << 8) + int(data[pos+3])
// 		if int(data[pos]) == 0x00 && int(data[pos+1]) == 0x00 {
// 			if pos+4+nextLen > len(data) {
// 				log.Printf("Less data 7 with err:%v", ErrTLSClientHello)
// 				return "", 0, ErrTLSClientHello
// 			}
// 			offset = offset + pos + 4
// 			return parseServerNameExtension(data[pos+4:], offset)
// 		}
// 		pos = pos + 4 + nextLen
// 	}
// 	if pos != len(data) {
// 		log.Printf("Less data 8 with err:%v", ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	return "", 0, ErrNoSNI
// }

// func parseServerNameExtension(data []byte, offset int) (string, int, error) {
// 	pos := 2
// 	for pos+3 < len(data) {
// 		nextLen := (int(data[pos+1]) << 8) + int(data[pos+2])
// 		if pos+3+nextLen > len(data) {
// 			log.Printf("Less data 9 with err:%v", ErrTLSClientHello)
// 			return "", 0, ErrTLSClientHello
// 		}

// 		if int(data[pos]) == 0x00 {
// 			offset = offset + pos + 1
// 			name := make([]byte, nextLen)
// 			copy(name, data[pos+3:])
// 			return string(name), offset, nil
// 		}
// 		pos = pos + 3 + nextLen
// 	}
// 	if pos != len(data) {
// 		log.Printf("Less data 10 with err:%v", ErrTLSClientHello)
// 		return "", 0, ErrTLSClientHello
// 	}
// 	return "", 0, ErrNoSNI
// }

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

var privateIPRanges [][]net.IP

func init() {
	range1 := []net.IP{net.ParseIP("192.168.0.0"), net.ParseIP("192.168.255.255")}
	range2 := []net.IP{net.ParseIP("172.16.0.0"), net.ParseIP("172.31.255.255")}
	range3 := []net.IP{net.ParseIP("10.0.0.0"), net.ParseIP("10.255.255.255")}
	privateIPRanges = append(privateIPRanges, range1)
	privateIPRanges = append(privateIPRanges, range2)
	privateIPRanges = append(privateIPRanges, range3)
}

func IsPrivateIP(ip string) bool {
	if strings.EqualFold(ip, "localhost") {
		return true
	}
	trial := net.ParseIP(ip)
	if trial.To4() == nil {
		return false
	}
	if strings.HasPrefix(ip, "127.0") {
		return true
	}
	for _, r := range privateIPRanges {
		if bytes.Compare(trial, r[0]) >= 0 && bytes.Compare(trial, r[1]) <= 0 {
			return true
		}
	}
	return false
}

func HTTPProxyConnect(proxyURL *url.URL, c net.Conn, addr string) error {
	connReq, err := http.NewRequest("CONNECT", "//"+addr, nil)
	if err != nil {
		return err
	}
	err = connReq.Write(c)
	if err != nil {
		return err
	}
	connRes, err := http.ReadResponse(bufio.NewReader(c), connReq)
	if err != nil {
		var tmp bytes.Buffer
		connReq.Write(&tmp)
		logger.Error("CONNECT %v error with request %s", proxyURL, string(tmp.Bytes()))
		return err
	}
	if nil != connRes.Body {
		connRes.Body.Close()
	}
	if connRes.StatusCode >= 300 {
		return fmt.Errorf("Invalid Connect response:%d %v %v", connRes.StatusCode, connRes, connReq)
	}
	return nil
}

func ProxyDial(proxyURL string, laddr, raddr string, timeout time.Duration, reuse bool) (net.Conn, error) {
	u, err := url.Parse(proxyURL)
	if nil != err {
		return nil, err
	}
	var c net.Conn
	if len(laddr) > 0 || reuse {
		opt := &protector.NetOptions{
			ReusePort:   reuse,
			LocalAddr:   laddr,
			DialTimeout: timeout,
		}
		c, err = protector.DialContextOptions(context.Background(), "tcp", u.Host, opt)
		//logger.Info("#####C %v with err:%v", u.Host, err)
	} else {
		c, err = netx.DialTimeout("tcp", u.Host, timeout)
	}

	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http":
		fallthrough
	case "https":
		if timeout > 0 {
			c.SetReadDeadline(time.Now().Add(timeout))
		}
		err = HTTPProxyConnect(u, c, raddr)
		if nil == err {
			if timeout > 0 {
				var zero time.Time
				c.SetReadDeadline(zero)
			}
		}
	case "socks":
	case "socks5":
		err = Socks5ProxyConnect(u, c, raddr)
	default:
		return nil, fmt.Errorf("invalid proxy schema:%s", u.Scheme)
	}
	if nil != err {
		c.Close()
		return nil, err
	}
	return c, nil
}

const socks5Version = 5

const (
	socks5AuthNone     = 0
	socks5AuthPassword = 2
)

const socks5Connect = 1

const (
	socks5IP4    = 1
	socks5Domain = 3
	socks5IP6    = 4
)

var socks5Errors = []string{
	"",
	"general failure",
	"connection forbidden",
	"network unreachable",
	"host unreachable",
	"connection refused",
	"TTL expired",
	"command not supported",
	"address type not supported",
}

func Socks5ProxyConnect(proxyURL *url.URL, conn net.Conn, addr string) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return errors.New("proxy: failed to parse port number: " + portStr)
	}
	if port < 1 || port > 0xffff {
		return errors.New("proxy: port number out of range: " + portStr)
	}

	// the size here is just an estimate
	buf := make([]byte, 0, 6+len(host))

	buf = append(buf, socks5Version)
	if nil != proxyURL.User && len(proxyURL.User.Username()) > 0 && len(proxyURL.User.Username()) < 256 {
		buf = append(buf, 2 /* num auth methods */, socks5AuthNone, socks5AuthPassword)
	} else {
		buf = append(buf, 1 /* num auth methods */, socks5AuthNone)
	}

	if _, err := conn.Write(buf); err != nil {
		return errors.New("proxy: failed to write greeting to SOCKS5 proxy at " + addr + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return errors.New("proxy: failed to read greeting from SOCKS5 proxy at " + addr + ": " + err.Error())
	}
	if buf[0] != 5 {
		return errors.New("proxy: SOCKS5 proxy at " + addr + " has unexpected version " + strconv.Itoa(int(buf[0])))
	}
	if buf[1] == 0xff {
		return errors.New("proxy: SOCKS5 proxy at " + addr + " requires authentication")
	}

	if buf[1] == socks5AuthPassword {
		buf = buf[:0]
		passwd, _ := proxyURL.User.Password()
		buf = append(buf, 1 /* password protocol version */)
		buf = append(buf, uint8(len(proxyURL.User.Username())))
		buf = append(buf, proxyURL.User.Username()...)
		buf = append(buf, uint8(len(passwd)))
		buf = append(buf, passwd...)

		if _, err := conn.Write(buf); err != nil {
			return errors.New("proxy: failed to write authentication request to SOCKS5 proxy at " + addr + ": " + err.Error())
		}

		if _, err := io.ReadFull(conn, buf[:2]); err != nil {
			return errors.New("proxy: failed to read authentication reply from SOCKS5 proxy at " + addr + ": " + err.Error())
		}

		if buf[1] != 0 {
			return errors.New("proxy: SOCKS5 proxy at " + addr + " rejected username/password")
		}
	}

	buf = buf[:0]
	buf = append(buf, socks5Version, socks5Connect, 0 /* reserved */)

	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			buf = append(buf, socks5IP4)
			ip = ip4
		} else {
			buf = append(buf, socks5IP6)
		}
		buf = append(buf, ip...)
	} else {
		if len(host) > 255 {
			return errors.New("proxy: destination hostname too long: " + host)
		}
		buf = append(buf, socks5Domain)
		buf = append(buf, byte(len(host)))
		buf = append(buf, host...)
	}
	buf = append(buf, byte(port>>8), byte(port))

	if _, err := conn.Write(buf); err != nil {
		return errors.New("proxy: failed to write connect request to SOCKS5 proxy at " + addr + ": " + err.Error())
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return errors.New("proxy: failed to read connect reply from SOCKS5 proxy at " + addr + ": " + err.Error())
	}

	failure := "unknown error"
	if int(buf[1]) < len(socks5Errors) {
		failure = socks5Errors[buf[1]]
	}

	if len(failure) > 0 {
		return errors.New("proxy: SOCKS5 proxy at " + addr + " failed to connect: " + failure)
	}

	bytesToDiscard := 0
	switch buf[3] {
	case socks5IP4:
		bytesToDiscard = net.IPv4len
	case socks5IP6:
		bytesToDiscard = net.IPv6len
	case socks5Domain:
		_, err := io.ReadFull(conn, buf[:1])
		if err != nil {
			return errors.New("proxy: failed to read domain length from SOCKS5 proxy at " + addr + ": " + err.Error())
		}
		bytesToDiscard = int(buf[0])
	default:
		return errors.New("proxy: got unknown address type " + strconv.Itoa(int(buf[3])) + " from SOCKS5 proxy at " + addr)
	}

	if cap(buf) < bytesToDiscard {
		buf = make([]byte, bytesToDiscard)
	} else {
		buf = buf[:bytesToDiscard]
	}
	if _, err := io.ReadFull(conn, buf); err != nil {
		return errors.New("proxy: failed to read address from SOCKS5 proxy at " + addr + ": " + err.Error())
	}

	// Also need to discard the port number
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return errors.New("proxy: failed to read port from SOCKS5 proxy at " + addr + ": " + err.Error())
	}
	return nil
}

func Socks5ProxyDial(proxyURL string, addr string, timeout time.Duration) (net.Conn, error) {
	u, err := url.Parse(proxyURL)
	if nil != err {
		return nil, err
	}
	c, err := netx.DialTimeout("tcp", u.Host, timeout)
	if err != nil {
		return nil, err
	}
	err = Socks5ProxyConnect(u, c, addr)
	if nil != err {
		c.Close()
		return nil, err
	}
	return c, nil
}

var localIPSet = make(map[string]bool)
var localIPv4 []string

func GetLocalIPv4() []string {
	if len(localIPv4) > 0 {
		return localIPv4
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		logger.Error("[ERROR]Failed to get local ip:%v", err)
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
func GetLocalIPSet() map[string]bool {
	if len(localIPSet) > 0 {
		return localIPSet
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		logger.Error("[ERROR]Failed to get local ip:%v", err)
		return localIPSet
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok {
			localIPSet[ipnet.IP.String()] = true
		}
	}
	return localIPSet
}

// Setup a bare-bones TLS config for the server
func GenerateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}
}

func IsConnClosed(c net.Conn) error {
	zero := []byte{}
	if _, err := c.Read(zero); nil != err {
		return err
	}
	return nil
}
