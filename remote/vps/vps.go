package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gotoolkit/ots"
	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/remote"
)

var totalConn int32

func serveProxyConn(conn net.Conn) {
	atomic.AddInt32(&totalConn, 1)
	bufconn := bufio.NewReader(conn)

	deferFunc := func() {
		conn.Close()
		atomic.AddInt32(&totalConn, -1)
	}
	defer deferFunc()

	ctx := remote.NewConnContext()

	// writeEvent := func(ev event.Event) error {
	// 	var buf bytes.Buffer
	// 	event.EncryptEvent(&buf, ev, ctx.IV)
	// 	conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	// 	_, err := conn.Write(buf.Bytes())
	// 	return err
	// }
	writeEvents := func(evs []event.Event) error {
		var buf bytes.Buffer
		for _, ev := range evs {
			event.EncryptEvent(&buf, ev, ctx.IV)
		}
		conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		_, err := conn.Write(buf.Bytes())
		return err
	}

	var buf bytes.Buffer
	b := make([]byte, 8192)

	var queue *remote.ConnEventQueue
	connClosed := false
	defer remote.ReleaseEventQueue(queue)
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
			writeEvents(ress)
			if nil == queue && len(ctx.User) > 0 && ctx.ConnIndex >= 0 {
				queue = remote.GetEventQueue(ctx.ConnId, true)
				go func() {
					var lastEventTime time.Time
					for !connClosed {
						evs, err := queue.PeekMulti(1 * time.Millisecond)
						if nil != err {
							if remote.GetSessionTableSize() > 0 && lastEventTime.Add(5*time.Second).Before(time.Now()) {
								evs = []event.Event{&event.HeartBeatEvent{}}
							} else {
								continue
							}
						}
						err = writeEvents(evs)
						lastEventTime = time.Now()
						if nil != err {
							log.Printf("TCP write error:%v", err)
							conn.Close()
							return
						} else {
							queue.DiscardPeeks()
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

func dumpServerStat(args []string, c io.Writer) error {
	fmt.Fprintf(c, "NumSession:    %d\n", remote.GetSessionTableSize())
	fmt.Fprintf(c, "NumEventQueue: %d\n", remote.GetEventQueueSize())
	fmt.Fprintf(c, "TotalUserConn: %d\n", totalConn)
	return nil
}

func main() {
	ots.RegisterHandler("vstat", dumpServerStat, 0, 0, "VStat                                 Dump server stat")
	err := ots.StartTroubleShootingServer(remote.ServerConf.AdminListen)
	if nil != err {
		log.Printf("Failed to start admin server with reason:%v", err)
		return
	}
	startLocalProxyServer(remote.ServerConf.Listen)
}
