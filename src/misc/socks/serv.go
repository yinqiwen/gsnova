package socks

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	//"log"
	"net"
	"sync"
)

var connections = new(sync.WaitGroup)

type Dialer interface {
	DialTCP(net string, laddr *net.TCPAddr, raddr string) (net.Conn, error)
}

func ServConn(local_reader *bufio.Reader, local *net.TCPConn, dialer Dialer) error {
	connections.Add(1)
	defer local.Close()
	defer connections.Done()

	// SOCKS does not include a length in the header, so take
	// a punt that each request will be readable in one go.
	buf := make([]byte, 256)
	n, err := local_reader.Read(buf)
	if err != nil || n < 2 {
		//log.Printf("[%s] unable to read SOCKS header: %v", local.RemoteAddr(), err)
		return fmt.Errorf("[%s] unable to read SOCKS header: %v", local.RemoteAddr(), err)
	}
	buf = buf[:n]

	switch version := buf[0]; version {
	case 4:
		switch command := buf[1]; command {
		case 1:
			port := binary.BigEndian.Uint16(buf[2:4])
			ipb := buf[4:8]
			ip := net.IP(buf[4:8])
			addr := (&net.TCPAddr{IP: ip, Port: int(port)}).String()
			if buf[4] == 0 && buf[5] == 0 && buf[6] == 0 {
				//socks4a
			}

			buf := buf[8:]
			i := bytes.Index(buf, []byte{0})
			if i < 0 {
				return fmt.Errorf("[%s] unable to locate SOCKS4 user", local.RemoteAddr())
			}
			//user := buf[:i]
			sock4a := false
			if len(buf) > i+1 {
				buf = buf[i+1:]
				i := bytes.Index(buf, []byte{0})
				if i < 0 {
					return fmt.Errorf("[%s] unable to locate SOCKS4a domain", local.RemoteAddr())
				}
				sock4a = true
				domain := buf[:i]
				addr = net.JoinHostPort(string(domain), fmt.Sprintf("%d", int(port)))
			}
			//log.Printf("[%s] incoming SOCKS4 TCP/IP stream connection, user=%q, raddr=%s", local.RemoteAddr(), user, addr)
			remote, err := dialer.DialTCP("tcp4", local.RemoteAddr().(*net.TCPAddr), addr)
			if err != nil {
				local.Write([]byte{0, 0x5b, 0, 0, 0, 0, 0, 0})
				return fmt.Errorf("[%s] unable to connect to remote host: %v", local.RemoteAddr(), err)
			}
			if sock4a {
				h := []byte{0, 0x5a}
				h = append(h, byte(port>>8), byte(port))
				h = append(h, ipb...)
				local.Write(h)
			} else {
				local.Write([]byte{0, 0x5a, 0, 0, 0, 0, 0, 0})
			}
			transfer(local, remote)
		default:
			return fmt.Errorf("[%s] unsupported command, closing connection", local.RemoteAddr())
		}
	case 5:
		authlen, buf := buf[1], buf[2:]
		auths, buf := buf[:authlen], buf[authlen:]
		if !bytes.Contains(auths, []byte{0}) {
			local.Write([]byte{0x05, 0xff})
			return fmt.Errorf("[%s] unsuported SOCKS5 authentication method", local.RemoteAddr())
		}
		local.Write([]byte{0x05, 0x00})
		buf = make([]byte, 256)
		n, err := local_reader.Read(buf)
		if err != nil {
			return fmt.Errorf("[%s] unable to read SOCKS header: %v", local.RemoteAddr(), err)
		}
		buf = buf[:n]
		switch version := buf[0]; version {
		case 5:
			switch command := buf[1]; command {
			case 1:
				buf = buf[3:]
				switch addrtype := buf[0]; addrtype {
				case 1:
					if len(buf) < 8 {
						local.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
						return fmt.Errorf("[%s] corrupt SOCKS5 TCP/IP stream connection request", local.RemoteAddr())
					}
					ip := net.IP(buf[1:5])
					port := binary.BigEndian.Uint16(buf[5:6])
					addr := &net.TCPAddr{IP: ip, Port: int(port)}
					//log.Printf("[%s] incoming SOCKS5 TCP/IP stream connection, raddr=%s", local.RemoteAddr(), addr)
					remote, err := dialer.DialTCP("tcp", local.RemoteAddr().(*net.TCPAddr), addr.String())
					if err != nil {
						local.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
						return fmt.Errorf("[%s] unable to connect to remote host: %v", local.RemoteAddr(), err)
					}
					local.Write([]byte{0x05, 0x00, 0x00, 0x01, ip[0], ip[1], ip[2], ip[3], byte(port >> 8), byte(port)})
					transfer(local, remote)
				case 3:
					addrlen, buf := buf[1], buf[2:]
					name, buf := buf[:addrlen], buf[addrlen:]
					//ip, err := net.ResolveIPAddr("tcp", string(name))
					//if err != nil {
					//	local.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
					//	return fmt.Errorf("[%s] unable to resolve IP address: %q, %v", local.RemoteAddr(), name, err)
					//}
					port := binary.BigEndian.Uint16(buf[:2])
					//addr := &net.TCPAddr{ip.IP, int(port)}
					addr := net.JoinHostPort(string(name), fmt.Sprintf("%d", int(port)))
					remote, err := dialer.DialTCP("tcp", local.RemoteAddr().(*net.TCPAddr), addr)
					if err != nil {
						local.Write([]byte{0x05, 0x04, 0x00, 0x03, 0, 0, 0, 0, 0, 0})
						return fmt.Errorf("[%s] unable to connect to remote host: %v", local.RemoteAddr(), err)
					}
					h := []byte{0x05, 0x00, 0x00, 0x03}
					h = append(h, addrlen)
					h = append(h, name...)
					h = append(h, byte(port>>8), byte(port))
					local.Write(h)
					transfer(local, remote)

				default:

					local.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
					return fmt.Errorf("[%s] unsupported SOCKS5 address type: %d", local.RemoteAddr(), addrtype)
				}
			default:

				local.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
				return fmt.Errorf("[%s] unknown SOCKS5 command: %d", local.RemoteAddr(), command)
			}
		default:
			local.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
			return fmt.Errorf("[%s] unnknown version after SOCKS5 handshake: %d", local.RemoteAddr(), version)
		}
	default:
		return fmt.Errorf("[%s] unknown SOCKS version: %d", local.RemoteAddr(), version)
	}
	return nil
}

func transfer(in, out net.Conn) {
	wg := new(sync.WaitGroup)
	wg.Add(2)
	f := func(in, out net.Conn, wg *sync.WaitGroup) {
		io.Copy(out, in)
		//log.Printf("xfer done, in=%v, out=%v, transfered=%d, err=%v", in.RemoteAddr(), out.RemoteAddr(), n, err)
		if conn, ok := in.(*net.TCPConn); ok {
			conn.CloseWrite()
		}
		if conn, ok := out.(*net.TCPConn); ok {
			conn.CloseRead()
		}
		wg.Done()
	}
	go f(in, out, wg)
	f(out, in, wg)
	wg.Wait()
	out.Close()
}
