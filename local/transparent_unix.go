// +build linux,!android

package local

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/common/netx"
	"github.com/yinqiwen/gsnova/common/protector"
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

type tudpSession struct {
	local  syscall.Sockaddr
	remote syscall.Sockaddr
	conf   *ProxyConfig
	stream mux.MuxStream

	key        string
	remoteIP   net.IP
	localIP    net.IP
	remotePort string
	localPort  string
}

func (t *tudpSession) close(err error) {
	if nil != t.stream {
		t.stream.Close()
	}
	tudpSessions.Delete(t.key)
	if nil != err {
		logger.Debug("Close transparent udp session:%s for reason:%v", t.key, err)
	}
}

func (t *tudpSession) handle(p []byte) {
	if nil == t.stream {
		if t.remoteIP.IsMulticast() {
			t.close(nil)
			return
		}
		protocol := "udp"
		isDNS := false
		if t.remotePort == "53" {
			protocol = "dns"
			isDNS = true
		}
		proxyChannelName := t.conf.getProxyChannelByHost(protocol, t.remoteIP.String())
		if len(proxyChannelName) == 0 {
			logger.Error("[ERROR]No proxy found for %s:%s", protocol, t.remoteIP.String())
			t.close(nil)
			return
		}
		logger.Debug("Select %s to proxy udp packet to %s:%s", proxyChannelName, t.remoteIP.String(), t.remotePort)
		stream, conf, err := channel.GetMuxStreamByChannel(proxyChannelName)
		var readTimeout int
		if nil == err {
			readTimeout = conf.RemoteDNSReadMSTimeout
			if isDNS {
				readTimeout = conf.RemoteDNSReadMSTimeout
			}
		}
		if nil != stream {
			opt := mux.StreamOptions{
				DialTimeout: conf.RemoteDialMSTimeout,
				ReadTimeout: readTimeout,
			}
			err = stream.Connect("udp", net.JoinHostPort(t.remoteIP.String(), t.remotePort), opt)
		}
		if nil != err || nil == stream {
			logger.Error("Failed to open stream for reason:%v by proxy:%s", err, proxyChannelName)
			t.close(err)
			return
		}

		t.stream = stream
		//u.streamReader, u.streamWriter = mux.GetCompressStreamReaderWriter(stream, conf.Compressor)
		go func() {
			b := make([]byte, 8192)
			var uerr error
			for {
				stream.SetReadDeadline(time.Now().Add(time.Duration(readTimeout) * time.Millisecond))
				n, err := stream.Read(b)
				if n > 0 {
					err = writeBackUDPData(b[0:n], t.local, t.remote)
				}
				uerr = err
				if nil != err {
					break
				}
				if isDNS {
					break
				}
			}
			t.close(uerr)
		}()
	}
	if nil == t.stream {
		t.close(nil)
		return
	}
	t.stream.Write(p)
}

var tudpSessions sync.Map

func getTUDPSession(proxy *ProxyConfig, laddr, raddr syscall.Sockaddr) *tudpSession {
	t := &tudpSession{
		local:  laddr,
		remote: raddr,
		conf:   proxy,
	}
	if _, ok := raddr.(*syscall.SockaddrInet4); ok {
		t.remotePort = fmt.Sprintf("%d", raddr.(*syscall.SockaddrInet4).Port)
		t.localPort = fmt.Sprintf("%d", laddr.(*syscall.SockaddrInet4).Port)
		t.remoteIP = make(net.IP, net.IPv4len)
		copy(t.remoteIP, raddr.(*syscall.SockaddrInet4).Addr[:])
		t.localIP = make(net.IP, net.IPv4len)
		copy(t.localIP, laddr.(*syscall.SockaddrInet4).Addr[:])
	} else {
		t.remotePort = fmt.Sprintf("%d", raddr.(*syscall.SockaddrInet6).Port)
		t.localPort = fmt.Sprintf("%d", laddr.(*syscall.SockaddrInet6).Port)
		t.remoteIP = make(net.IP, net.IPv6len)
		copy(t.remoteIP, raddr.(*syscall.SockaddrInet6).Addr[:])
		t.localIP = make(net.IP, net.IPv6len)
		copy(t.localIP, laddr.(*syscall.SockaddrInet6).Addr[:])
	}
	t.key = fmt.Sprintf("%s:%s->%s:%s", t.localIP.String(), t.localPort, t.remoteIP.String(), t.remotePort)
	actual, _ := tudpSessions.LoadOrStore(t.key, t)
	return actual.(*tudpSession)
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
		u := getTUDPSession(proxy, local, remote)
		u.handle(data)
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

func supportTransparentProxy() bool {
	return true
}
