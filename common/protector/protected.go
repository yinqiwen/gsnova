// +build !windows

// Package protected is used for creating "protected" connections
// that bypass Android's VpnService
package protector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	SO_MARK = 0x24
)

var SocketMark int

func SupportReusePort() bool {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if nil != err {
		return false
	}
	defer syscall.Close(fd)
	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	if nil != err {
		return false
	}
	return true
}

type ProtectedConnBase struct {
	mutex    sync.Mutex
	isClosed bool
	socketFd int
	ip       net.IP
	//ip       [4]byte
	port int
}

// converts the protected connection specified by
// socket fd to a net.Conn
func (conn *ProtectedConnBase) convertLisener() (net.Listener, error) {
	conn.mutex.Lock()
	file := os.NewFile(uintptr(conn.socketFd), "")
	// dup the fd and return a copy
	fileLis, err := net.FileListener(file)
	// closes the original fd
	file.Close()
	conn.socketFd = socketError
	if err != nil {
		conn.mutex.Unlock()
		return nil, err
	}
	conn.mutex.Unlock()
	return fileLis, nil
}

func (conn *ProtectedConnBase) bindSocket(ip net.IP, port int, ipv6 bool) error {
	var sockAddr syscall.Sockaddr
	if !ipv6 {
		sockAddr4 := &syscall.SockaddrInet4{Port: port}
		copy(sockAddr4.Addr[:], ip.To4()[0:4])
		sockAddr = sockAddr4
	} else {
		sockAddr6 := &syscall.SockaddrInet6{Port: port}
		copy(sockAddr6.Addr[:], ip.To16()[0:16])
		sockAddr = sockAddr6
	}
	err := syscall.Bind(conn.socketFd, sockAddr)
	if nil != err {
		return err
	}
	return err
}

// cleanup is ran whenever we encounter a socket error
// we use a mutex since this connection is active in a variety
// of goroutines and to prevent any possible race conditions
func (conn *ProtectedConnBase) cleanup() {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if conn.socketFd != socketError {
		//fmt.Printf("####Close %d\n", conn.socketFd)
		syscall.Close(conn.socketFd)
		conn.socketFd = socketError
	}
}

// connectSocket makes the connection to the given IP address port
// for the given socket fd
func (conn *ProtectedConnBase) connectSocket(timeout time.Duration) error {
	if nil == conn.ip {
		return fmt.Errorf("Empty IP to connect")
	}
	var sockAddr syscall.Sockaddr
	//bindAddr := &syscall.SockaddrInet4{Port: 0}
	// lip := net.ParseIP("27.38.173.96")
	// copy(bindAddr.Addr[:], lip[0:4])
	if conn.ip.To4() != nil {
		sockAddr4 := &syscall.SockaddrInet4{Port: conn.port}
		copy(sockAddr4.Addr[:], conn.ip.To4()[:])
		sockAddr = sockAddr4
	} else {
		sockAddr6 := &syscall.SockaddrInet6{Port: conn.port}
		copy(sockAddr6.Addr[:], conn.ip.To16()[0:16])
		sockAddr = sockAddr6
		//bindAddr = &syscall.SockaddrInet6{Port: 0}
	}
	// err := syscall.Bind(conn.socketFd, bindAddr)
	// if nil != err {
	// 	return err
	// }
	errCh := make(chan error, 2)
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	time.AfterFunc(timeout, func() {
		errCh <- errors.New("connect timeout")
	})

	go func() {
		errCh <- syscall.Connect(conn.socketFd, sockAddr)
	}()
	err := <-errCh
	return err
}

type ProtectedPacketConn struct {
	ProtectedConnBase
	net.PacketConn
	rawFile *os.File
}

func (c *ProtectedPacketConn) Write(b []byte) (int, error) {
	writer, ok := c.PacketConn.(io.Writer)
	if !ok {
		return 0, syscall.EINVAL
	}
	return writer.Write(b)
}

func (c *ProtectedPacketConn) Close() error {
	return c.PacketConn.Close()
}

// converts the protected connection specified by
// socket fd to a net.Conn
func (conn *ProtectedPacketConn) convert() error {
	conn.mutex.Lock()
	file := os.NewFile(uintptr(conn.socketFd), "")
	conn.rawFile = file
	// dup the fd and return a copy
	fileConn, err := net.FilePacketConn(file)
	// closes the original fd
	file.Close()
	conn.socketFd = socketError
	if err != nil {
		conn.mutex.Unlock()
		return err
	}
	conn.PacketConn = fileConn
	conn.mutex.Unlock()
	return nil
}

// connectSocket makes the connection to the given IP address port
// for the given socket fd
func (conn *ProtectedPacketConn) listenSocket(ip net.IP, port int, ipv6 bool) error {
	return conn.bindSocket(ip, port, ipv6)
}

type ProtectedConn struct {
	net.Conn
	ProtectedConnBase
}

// Resolve resolves the given address using a DNS lookup on a UDP socket
// protected by the currnet Protector.
func Resolve(network string, addr string) (*net.TCPAddr, error) {
	host, port, err := SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	// Check if we already have the IP address
	IPAddr := net.ParseIP(host)
	if IPAddr != nil {
		return &net.TCPAddr{IP: IPAddr, Port: port}, nil
	}
	// Create a datagram socket
	socketFd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, fmt.Errorf("Error creating socket: %v", err)
	}
	defer syscall.Close(socketFd)

	// Here we protect the underlying socket from the
	// VPN connection by passing the file descriptor
	// back to Java for exclusion
	err = currentProtect(socketFd)
	if err != nil {
		return nil, fmt.Errorf("Could not bind socket to system device: %v", err)
	}

	IPAddr = net.ParseIP(currentDnsServer)
	if IPAddr == nil {
		return nil, errors.New("invalid IP address")
	}

	var ip [4]byte
	copy(ip[:], IPAddr.To4())
	sockAddr := syscall.SockaddrInet4{Addr: ip, Port: dnsPort}

	err = syscall.Connect(socketFd, &sockAddr)
	if err != nil {
		return nil, err
	}

	fd := uintptr(socketFd)
	file := os.NewFile(fd, "")
	defer file.Close()

	// return a copy of the network connection
	// represented by file
	fileConn, err := net.FileConn(file)
	if err != nil {
		log.Printf("Error returning a copy of the network connection: %v", err)
		return nil, err
	}

	setQueryTimeouts(fileConn)

	//log.Printf("performing dns lookup...!!")
	result, err := DnsLookup(host, fileConn)
	if err != nil {
		log.Printf("Error doing DNS resolution: %v", err)
		return nil, err
	}
	ipAddr, err := result.PickRandomIP()
	if err != nil {
		log.Printf("No IP address available: %v", err)
		return nil, err
	}
	return &net.TCPAddr{IP: ipAddr, Port: port}, nil
}

func Dial(network, addr string, timeout time.Duration) (net.Conn, error) {
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	return DialContext(ctx, network, addr)
}

func setLinger(fd int, sec int) error {
	var l unix.Linger
	if sec >= 0 {
		l.Onoff = 1
		l.Linger = int32(sec)
	} else {
		l.Onoff = 0
		l.Linger = 0
	}
	return os.NewSyscallError("setsockopt", unix.SetsockoptLinger(fd, unix.SOL_SOCKET, unix.SO_LINGER, &l))
}

func reuseTCPAddr(conn *ProtectedConn) error {
	// if err := unix.SetsockoptInt(conn.socketFd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
	// 	unix.Close(conn.socketFd)
	// 	return err
	// }
	err := unix.SetsockoptInt(conn.socketFd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	if nil != err {
		syscall.Close(conn.socketFd)
		return err
	}
	// err = setLinger(conn.socketFd, 5)
	// if nil != err {
	// 	syscall.Close(conn.socketFd)
	// 	return err
	// }
	return nil
}

func dialContext(ctx context.Context, network, raddr string, options *NetOptions) (net.Conn, error) {
	host, port, err := SplitHostPort(raddr)
	if err != nil {
		log.Printf("###Split %v error:%v", raddr, err)
		return nil, err
	}

	conn := &ProtectedConn{}
	conn.ProtectedConnBase.port = port
	// do DNS query
	IPAddr := net.ParseIP(host)
	if IPAddr == nil {
		log.Printf("Couldn't parse IP address %v while port:%d", host, port)
		return nil, err
	}
	conn.ip = IPAddr

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	family := syscall.AF_INET
	if IPAddr.To4() == nil {
		family = syscall.AF_INET6
	}
	//copy(conn.ip[:], IPAddr.To4())
	var socketFd int
	isTCP := false
	switch network {
	case "udp", "udp4", "udp6":
		socketFd, err = syscall.Socket(family, syscall.SOCK_DGRAM, 0)
	default:
		socketFd, err = syscall.Socket(family, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
		isTCP = true
	}
	if nil == err && SocketMark > 0 {
		err = syscall.SetsockoptInt(socketFd, syscall.SOL_SOCKET, SO_MARK, SocketMark)
	}
	if err != nil {
		log.Printf("Could not create socket: %v", err)
		if socketFd > 0 {
			syscall.Close(socketFd)
		}
		return nil, err
	}
	err = syscall.SetsockoptInt(socketFd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	conn.socketFd = socketFd
	if nil != options {
		if isTCP && options.ReusePort {
			if nil != err {
				log.Printf("SO_REUSEADDR fail: %v", err)
				syscall.Close(socketFd)
				return nil, err
			}
			err = reuseTCPAddr(conn)
			if nil != err {
				log.Printf("reuseTCPAddr fail: %v", err)
				return nil, err
			}
		}
		if isTCP && len(options.LocalAddr) > 0 {
			ltcpAddr, err := net.ResolveTCPAddr("tcp", options.LocalAddr)
			if nil != err {
				log.Printf("ResolveTCPAddr %v fail: %v", options.LocalAddr, err)
				syscall.Close(socketFd)
				return nil, err
			}
			err = conn.bindSocket(ltcpAddr.IP, ltcpAddr.Port, ltcpAddr.IP.To4() == nil)
			if nil != err {
				syscall.Close(conn.socketFd)
				return nil, err
			}
		}
	}
	defer conn.cleanup()

	// Actually protect the underlying socket here
	err = currentProtect(conn.socketFd)
	if err != nil {
		return nil, fmt.Errorf("Could not bind socket to system device: %v", err)
	}
	var timeout time.Duration
	if nil != options {
		timeout = options.DialTimeout
	}
	err = conn.connectSocket(timeout)
	if err != nil {
		log.Printf("Could not connect to %s socket: %v %v", raddr, err, options)
		return nil, err
	}

	// finally, convert the socket fd to a net.Conn
	err = conn.convert()
	if err != nil {
		log.Printf("Error converting protected connection: %v", err)
		return nil, err
	}

	//conn.Conn.SetDeadline(time.Now().Add(timeout))
	return conn.Conn, nil
}

// Dial creates a new protected connection, it assumes that the address has
// already been resolved to an IPv4 address.
// - syscall API calls are used to create and bind to the
//   specified system device (this is primarily
//   used for Android VpnService routing functionality)
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return dialContext(ctx, network, addr, nil)
}

func DialContextOptions(ctx context.Context, network, addr string, opt *NetOptions) (net.Conn, error) {
	return dialContext(ctx, network, addr, opt)
}

func ListenTCP(laddr *net.TCPAddr, options *NetOptions) (net.Listener, error) {
	conn := &ProtectedConn{}
	conn.ProtectedConnBase.port = laddr.Port
	family := syscall.AF_INET
	if len(laddr.IP) > 0 && laddr.IP.To4() == nil {
		family = syscall.AF_INET6
	}
	socketFd, err := syscall.Socket(family, syscall.SOCK_STREAM, 0)
	if err != nil {
		log.Printf("Could not create tcp socket: %v", err)
		if socketFd > 0 {
			syscall.Close(socketFd)
		}
		return nil, err
	}
	syscall.SetsockoptInt(socketFd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	conn.socketFd = socketFd
	if nil != options {
		if options.ReusePort {
			err = reuseTCPAddr(conn)
			if err != nil {
				log.Printf("Could not reuse tcp socket: %v", err)
				if socketFd > 0 {
					syscall.Close(socketFd)
				}
				return nil, err
			}
		}
	}
	defer conn.cleanup()

	// Actually protect the underlying socket here
	err = currentProtect(conn.socketFd)
	if err != nil {
		return nil, fmt.Errorf("Could not bind socket to system device: %v", err)
	}
	if len(laddr.IP) > 0 {
		err = conn.bindSocket(laddr.IP, laddr.Port, family == syscall.AF_INET6)
		if err != nil {
			log.Printf("Could not bind listen tcp socket: %v", err)
			return nil, err
		}
	}

	err = syscall.Listen(conn.socketFd, 1024)
	if nil != err {
		log.Printf("Could not listen tcp socket: %v", err)
		return nil, err
	}

	// finally, convert the socket fd to a net.Conn
	var lis net.Listener
	lis, err = conn.convertLisener()
	if err != nil {
		log.Printf("Error converting protected listener: %v", err)
		return nil, err
	}
	return lis, nil
}

// converts the protected connection specified by
// socket fd to a net.Conn
func (conn *ProtectedConn) convert() error {
	conn.mutex.Lock()
	file := os.NewFile(uintptr(conn.socketFd), "")
	// dup the fd and return a copy
	fileConn, err := net.FileConn(file)
	// closes the original fd
	file.Close()
	conn.socketFd = socketError
	if err != nil {
		conn.mutex.Unlock()
		return err
	}
	conn.Conn = fileConn
	conn.mutex.Unlock()
	return nil
}

// Close is used to destroy a protected connection
func (conn *ProtectedConn) Close() (err error) {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if !conn.isClosed {
		conn.isClosed = true
		if conn.Conn == nil {
			if conn.socketFd == socketError {
				err = nil
			} else {
				err = syscall.Close(conn.socketFd)
				// update socket fd to socketError
				// to make it explicit this connection
				// has been closed
				conn.socketFd = socketError
			}
		} else {
			err = conn.Conn.Close()
		}
	}
	return err
}

// configure DNS query expiration
func setQueryTimeouts(c net.Conn) {
	now := time.Now()
	c.SetReadDeadline(now.Add(readDeadline))
	c.SetWriteDeadline(now.Add(writeDeadline))
}

// wrapper around net.SplitHostPort that also converts
// uses strconv to convert the port to an int
func SplitHostPort(addr string) (string, int, error) {
	host, sPort, err := net.SplitHostPort(addr)
	if err != nil {
		log.Printf("Could not split network address: %v", err)
		return "", 0, err
	}
	port, err := strconv.Atoi(sPort)
	if err != nil {
		log.Printf("No port number found %v", err)
		return "", 0, err
	}
	return host, port, nil
}

func ListenUDP(network string, laddr *net.UDPAddr) (net.PacketConn, error) {
	conn := &ProtectedPacketConn{}
	conn.ProtectedConnBase.port = laddr.Port
	family := syscall.AF_INET
	if laddr.IP.To4() == nil {
		family = syscall.AF_INET6
	}
	socketFd, err := syscall.Socket(family, syscall.SOCK_DGRAM, 0)
	if nil == err && SocketMark > 0 {
		err = syscall.SetsockoptInt(socketFd, syscall.SOL_SOCKET, SO_MARK, SocketMark)
	}
	if err != nil {
		log.Printf("Could not create udp socket: %v", err)
		if socketFd > 0 {
			syscall.Close(socketFd)
		}
		return nil, err
	}
	conn.socketFd = socketFd

	defer conn.cleanup()

	// Actually protect the underlying socket here
	err = currentProtect(conn.socketFd)
	if err != nil {
		return nil, fmt.Errorf("Could not bind socket to system device: %v", err)
	}

	err = conn.listenSocket(laddr.IP, laddr.Port, family == syscall.AF_INET6)
	if err != nil {
		log.Printf("Could not listen udp socket: %v", err)
		return nil, err
	}

	// finally, convert the socket fd to a net.Conn
	err = conn.convert()
	if err != nil {
		log.Printf("Error converting protected connection: %v", err)
		return nil, err
	}
	return conn.PacketConn, nil
}

func DialUDP(network string, laddr, raddr *net.UDPAddr) (net.PacketConn, error) {
	conn := &ProtectedPacketConn{}
	conn.ProtectedConnBase.port = raddr.Port
	conn.ip = raddr.IP
	family := syscall.AF_INET
	if raddr.IP.To4() == nil {
		family = syscall.AF_INET6
	}
	socketFd, err := syscall.Socket(family, syscall.SOCK_DGRAM, 0)
	if nil == err && SocketMark > 0 {
		err = syscall.SetsockoptInt(socketFd, syscall.SOL_SOCKET, SO_MARK, SocketMark)
	}
	if err != nil {
		log.Printf("Could not create udp socket: %v", err)
		if socketFd > 0 {
			syscall.Close(socketFd)
		}
		return nil, err
	}
	conn.socketFd = socketFd
	defer conn.cleanup()
	// Actually protect the underlying socket here
	err = currentProtect(conn.socketFd)
	if err != nil {
		return nil, fmt.Errorf("Could not bind socket to system device: %v", err)
	}
	// if nil != laddr {
	// 	conn.listenSocket(laddr.IP, laddr.Port, family == syscall.AF_INET6)
	// }
	var timeout time.Duration
	err = conn.connectSocket(timeout)
	if err != nil {
		log.Printf("Could not connect to %s socket: %v", raddr, err)
		return nil, err
	}

	// finally, convert the socket fd to a net.Conn
	err = conn.convert()
	if err != nil {
		log.Printf("Error converting protected connection: %v", err)
		return nil, err
	}

	//conn.Conn.SetDeadline(time.Now().Add(timeout))
	return conn.PacketConn, nil
}
