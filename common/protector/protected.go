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
)

const (
	SO_MARK = 0x24
)

var SocketMark int

type ProtectedConnBase struct {
	mutex    sync.Mutex
	isClosed bool
	socketFd int
	ip       net.IP
	//ip       [4]byte
	port int
}

// cleanup is ran whenever we encounter a socket error
// we use a mutex since this connection is active in a variety
// of goroutines and to prevent any possible race conditions
func (conn *ProtectedConnBase) cleanup() {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if conn.socketFd != socketError {
		syscall.Close(conn.socketFd)
		conn.socketFd = socketError
	}
}

// connectSocket makes the connection to the given IP address port
// for the given socket fd
func (conn *ProtectedConnBase) connectSocket() error {
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
	time.AfterFunc(connectTimeOut, func() {
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
	var sockAddr syscall.Sockaddr
	if !ipv6 {
		sockAddr4 := &syscall.SockaddrInet4{Port: conn.port}
		copy(sockAddr4.Addr[:], ip.To4()[0:4])
		sockAddr = sockAddr4
	} else {
		sockAddr6 := &syscall.SockaddrInet6{Port: conn.port}
		copy(sockAddr6.Addr[:], ip.To16()[0:16])
		sockAddr = sockAddr6
	}
	err := syscall.Bind(conn.socketFd, sockAddr)
	if nil != err {
		return err
	}
	// errCh := make(chan error, 2)
	// time.AfterFunc(connectTimeOut, func() {
	// 	errCh <- errors.New("connect timeout")
	// })
	// go func() {
	// 	errCh <- syscall.Listen(conn.socketFd, 512)
	// }()
	// err = <-errCh
	return err
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

// Dial creates a new protected connection, it assumes that the address has
// already been resolved to an IPv4 address.
// - syscall API calls are used to create and bind to the
//   specified system device (this is primarily
//   used for Android VpnService routing functionality)
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := SplitHostPort(addr)
	if err != nil {
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
	//var err error
	switch network {
	case "udp", "udp4", "udp6":
		socketFd, err = syscall.Socket(family, syscall.SOCK_DGRAM, 0)
	default:
		socketFd, err = syscall.Socket(family, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
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
	syscall.SetsockoptInt(socketFd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	conn.socketFd = socketFd

	defer conn.cleanup()

	// Actually protect the underlying socket here
	err = currentProtect(conn.socketFd)
	if err != nil {
		return nil, fmt.Errorf("Could not bind socket to system device: %v", err)
	}

	err = conn.connectSocket()
	if err != nil {
		log.Printf("Could not connect to %s socket: %v", addr, err)
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
	err = conn.connectSocket()
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
