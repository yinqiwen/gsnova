// +build linux

package proxy

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"syscall"
	"time"

	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/local/protector"
)

const (
	SO_ORIGINAL_DST      = 80
	IP6T_SO_ORIGINAL_DST = 80
	IPV6_RECVORIGDSTADDR = 74
)

func getOrinalTCPRemoteAddr(conn net.Conn) (net.Conn, net.IP, uint16, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, nil, 0, fmt.Errorf("Invalid connection with type:%T", conn)
	}

	clientConnFile, err := tcpConn.File()
	if err != nil {
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
		port = uint16(ipv4Addr.Multiaddr[2])<<8 + uint16(ipv4Addr.Multiaddr[3])
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

func startTransparentUDProxy(addr string, proxy *ProxyConfig) {
	lhost, lport, err := net.SplitHostPort(addr)
	if nil != err {
		logger.Error("Split error:%v", err)
		return
	}
	port, err := strconv.Atoi(lport)
	if nil != err {
		logger.Error("Split port error:%v", err)
		return
	}
	family := syscall.AF_INET
	var ip net.IP
	isIPv4 := true
	if len(lhost) > 0 {
		ip = net.ParseIP(lhost)
		if ip.To4() != nil {
			family = syscall.AF_INET
		} else {
			family = syscall.AF_INET6
			isIPv4 = false
		}
	} else {
		ip = net.IPv4zero
	}
	//logger.Debug("1 : %d %d %s", len(lhost), len(ip), lhost)
	socketFd, err := syscall.Socket(family, syscall.SOCK_DGRAM, 0)
	if nil != err {
		logger.Error("Failed to create udp listen socket:%v", err)
		return
	}
	syscall.SetsockoptInt(socketFd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	err = syscall.SetsockoptInt(socketFd, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
	if nil != err {
		logger.Error("Failed to set transparent udp  socket:%v", err)
		return
	}
	if isIPv4 {
		err = syscall.SetsockoptInt(socketFd, syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1)
	} else {
		err = syscall.SetsockoptInt(socketFd, syscall.SOL_IPV6, IPV6_RECVORIGDSTADDR, 1)
	}
	if nil != err {
		logger.Error("Failed to set socket opt 'RECVORIGDSTADDR' with reason:%v", err)
		return
	}
	var sockAddr syscall.Sockaddr
	if isIPv4 {
		addr4 := &syscall.SockaddrInet4{Port: port}
		sockAddr = addr4
		copy(addr4.Addr[:], ip.To4())
	} else {
		addr6 := &syscall.SockaddrInet6{Port: port}
		sockAddr = addr6
		copy(addr6.Addr[:], ip.To16())
	}

	err = syscall.Bind(socketFd, sockAddr)
	if nil != err {
		logger.Error("Bind udp socket  error:%v with addr:%v", err, sockAddr)
		return
	}
	logger.Info("Listen transparent UDP proxy on %s:%d", ip.String(), port)
	for proxyServerRunning {
		data, local, remote, err := recvTransparentUDP(socketFd)
		if nil != err {
			logger.Error("Recv msg error:%v", err)
			continue
		}

		go func(p []byte, laddr, raddr syscall.Sockaddr) {
			protocol := "udp"
			remoteHost := ""
			remotePort := ""
			if _, ok := raddr.(*syscall.SockaddrInet4); ok {
				remotePort = fmt.Sprintf("%d", raddr.(*syscall.SockaddrInet4).Port)
				ip := make(net.IP, net.IPv4len)
				copy(ip, raddr.(*syscall.SockaddrInet4).Addr[:])
				remoteHost = ip.String()
			} else {
				remotePort = fmt.Sprintf("%d", raddr.(*syscall.SockaddrInet6).Port)
				ip := make(net.IP, net.IPv6len)
				copy(ip, raddr.(*syscall.SockaddrInet6).Addr[:])
				remoteHost = ip.String()
			}
			readTimeout := time.Duration(proxy.UDPReadMSTimeout) * time.Millisecond
			if remotePort == "53" {
				protocol = "dns"
				readTimeout = time.Duration(proxy.DNSReadMSTimeout) * time.Millisecond
			}
			proxyChannelName := proxy.getProxyChannelByHost(protocol, remoteHost)
			if len(proxyChannelName) == 0 {
				logger.Error("[ERROR]No proxy found for %s:%s", protocol, remoteHost)
				return
			}
			logger.Debug("Select %s to proxy udp packet to %s:%s", proxyChannelName, remoteHost, remotePort)
			if proxyChannelName == directProxyChannelName {
				res, err := dnsQueryRaw(p, true)
				if nil == err {
					writeBackUDPData(res, laddr, raddr)
				}
				if nil != err {
					logger.Error("[ERROR]Failed to query dns with reason:%v", err)
				}
				return
			}
			stream, _, err := getMuxStreamByChannel(proxyChannelName)
			if nil != stream {
				err = stream.Connect("udp", net.JoinHostPort(remoteHost, remotePort))
			}
			if nil != err || nil == stream {
				logger.Error("Failed to open stream for reason:%v by proxy:%s", err, proxyChannelName)
				return
			}
			buf := make([]byte, 4096)
			stream.Write(p)
			stream.SetReadDeadline(time.Now().Add(readTimeout))
			n, _ := stream.Read(buf)
			if n > 0 {
				writeBackUDPData(buf[0:n], laddr, raddr)
			}
			stream.Close()
		}(data, local, remote)
	}
}

func recvTransparentUDP(fd int) ([]byte, syscall.Sockaddr, syscall.Sockaddr, error) {
	buf := make([]byte, 4096)
	oob := make([]byte, 64)
	n, cn, _, local, err := syscall.Recvmsg(fd, buf, oob, 0)
	if nil != err {
		return nil, nil, nil, err
	}
	ctrlMsgs, err := syscall.ParseSocketControlMessage(oob[0:cn])
	if nil != err {
		return nil, nil, nil, err
	}
	for _, cmsg := range ctrlMsgs {
		if cmsg.Header.Level == syscall.SOL_IP && cmsg.Header.Type == syscall.IP_RECVORIGDSTADDR {
			//memcpy(dstaddr, CMSG_DATA(cmsg), sizeof(struct sockaddr_in));
			//dstaddr->ss_family = AF_INET;
			remote := &syscall.SockaddrInet4{}
			remote.Port = int(cmsg.Data[2])<<8 + int(cmsg.Data[3])
			copy(remote.Addr[:], cmsg.Data[4:8])
			return buf[0:n], local, remote, nil
		} else if cmsg.Header.Level == syscall.SOL_IPV6 && cmsg.Header.Type == IPV6_RECVORIGDSTADDR {
			remote := &syscall.SockaddrInet6{}
			remote.Port = int(cmsg.Data[2])<<8 + int(cmsg.Data[3])
			copy(remote.Addr[:], cmsg.Data[8:24])
			return buf[0:n], local, remote, nil
		}
	}
	return nil, nil, nil, fmt.Errorf("Can NOT get orgin remote address")
}

func writeBackUDPData(data []byte, local, remote syscall.Sockaddr) error {
	family := syscall.AF_INET
	if _, ok := local.(*syscall.SockaddrInet6); ok {
		family = syscall.AF_INET6
	}
	socketFd, err := syscall.Socket(family, syscall.SOCK_DGRAM, 0)
	if err != nil {
		log.Printf("Could not create udp socket: %v", err)
		return err
	}
	defer syscall.Close(socketFd)
	err = syscall.SetsockoptInt(socketFd, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
	if nil == err {
		syscall.SetsockoptInt(socketFd, syscall.SOL_IP, syscall.SO_REUSEADDR, 1)
	} else {
		return err
	}
	err = syscall.Bind(socketFd, remote)
	if nil != err {
		return err
	}
	err = syscall.Sendto(socketFd, data, 0, local)
	return err
}

func enableTransparentSocketMark(v int) {
	protector.SocketMark = v
	netx.OverrideDial(protector.DialContext)
	netx.OverrideListenUDP(protector.ListenUDP)
	netx.OverrideDialUDP(protector.DialUDP)
}
