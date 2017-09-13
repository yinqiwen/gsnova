// +build !windows

package proxy

import (
	"fmt"
	"log"
	"net"
	"syscall"
)

const (
	SO_ORIGINAL_DST      = 80
	IP6T_SO_ORIGINAL_DST = 80
)

func getOrinalTCPRemoteAddr(conn net.Conn) (net.Conn, net.IP, uint16, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, nil, 0, fmt.Errorf("Invalid connection with type:%T", conn)
	}

	clientConnFile, err := tcpConn.File()
	if err != nil {
		log.Printf("####%v", err)
		return nil, nil, 0, err
	} else {
		tcpConn.Close()
	}
	fd := int(clientConnFile.Fd())
	var port uint16
	var ip net.IP
	//the trick way to get orginal ip/port by syscall
	ipv6Addr, err := syscall.GetsockoptIPv6MTUInfo(fd, syscall.IPPROTO_IPV6, IP6T_SO_ORIGINAL_DST)
	if err != nil {
		ipv4Addr, err := syscall.GetsockoptIPv6Mreq(fd, syscall.IPPROTO_IP, SO_ORIGINAL_DST)
		if nil != err {
			clientConnFile.Close()
			return nil, nil, 0, err
		}
		port = uint16(addr.Multiaddr[2])<<8 + uint16(addr.Multiaddr[3])
		ip = net.IPv4(ipv4Addr.Multiaddr[4], ipv4Addr.Multiaddr[5], ipv4Addr.Multiaddr[6], ipv4Addr.Multiaddr[7])
	} else {
		port = ipv6Addr.Addr.Port
		ip = make(net.IP, net.IPv6len)
		copy(ip, ipv6Addr.Addr.Addr[:])
	}

	newConn, err := net.FileConn(clientConnFile)
	if err != nil {
		clientConnFile.Close()
		return nil, nil, 0, err
	}

	return newConn, ip, port, nil
}
