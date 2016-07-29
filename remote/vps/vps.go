package main

import (
	"bufio"
	"bytes"
	"log"
	"net"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/remote"
)

func serveProxyConn(conn net.Conn) {
	bufconn := bufio.NewReader(conn)
	defer conn.Close()

	ctx := &remote.ConnContex{}

	writeEvent := func(ev event.Event) error {
		var buf bytes.Buffer
		event.EncryptEvent(&buf, ev, ctx.IV)
		_, err := conn.Write(buf.Bytes())
		return err
	}

	var buf bytes.Buffer
	b := make([]byte, 8192)

	var queue *event.EventQueue
	connClosed := false
	for !connClosed {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, cerr := bufconn.Read(b)
		if n > 0 {
			buf.Write(b[0:n])
		}
		if nil != cerr {
			conn.Close()
			connClosed = true
		}
		ress, err := remote.HandleRequestBuffer(&buf, ctx)
		if nil != err {
			if err != event.EBNR {
				log.Printf("[ERROR]connection %s:%d error:%v", ctx.User, ctx.ConnIndex, err)
				conn.Close()
				connClosed = true
				return
			}
		} else {
			for _, res := range ress {
				writeEvent(res)
			}
			if nil == queue && len(ctx.User) > 0 && ctx.ConnIndex >= 0 {
				queue = remote.GetEventQueue(ctx.ConnId, true)
				go func() {
					for !connClosed {
						ev, err := queue.Peek(1 * time.Millisecond)
						if nil != err {
							continue
						}
						err = writeEvent(ev)
						if nil != err {
							log.Printf("TCP write error:%v", err)
							conn.Close()
							return
						} else {
							queue.ReadPeek()
						}
					}
				}()
			}
		}
	}
}

func startLocalProxyServer(addr string) error {
	if len(addr) == 0 {
		log.Fatalf("Empty listen address.")
		return nil
	}
	tcpaddr, err := net.ResolveTCPAddr("tcp", addr)
	if nil != err {
		log.Fatalf("[ERROR]Local server address:%s error:%v", addr, err)
		return err
	}
	var lp *net.TCPListener
	lp, err = net.ListenTCP("tcp", tcpaddr)
	if nil != err {
		log.Fatalf("Can NOT listen on address:%s", addr)
		return err
	}
	log.Printf("Listen on address %s", addr)
	for {
		conn, err := lp.AcceptTCP()
		if nil != err {
			continue
		}
		go serveProxyConn(conn)
	}
	return nil
}

func main() {
	startLocalProxyServer(remote.ServerConf.Listen)
}
