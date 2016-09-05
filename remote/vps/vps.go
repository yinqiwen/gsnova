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
	"github.com/yinqiwen/gsnova/common/helper"
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
	writeEvents := func(evs []event.Event, buf *bytes.Buffer) error {
		buf.Reset()
		if len(evs) > 0 {
			//var buf bytes.Buffer
			for _, ev := range evs {
				if nil != ev {
					event.EncryptEvent(buf, ev, ctx.IV)
				}
			}
			if buf.Len() > 0 {
				conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
				_, err := conn.Write(buf.Bytes())
				return err
			}
			return nil

		} else {
			return nil
		}
	}

	var rbuf bytes.Buffer
	var wbuf bytes.Buffer
	//b := make([]byte, 8192)

	writeTaskRunning := false
	connClosed := false
	reader := &helper.BufferChunkReader{bufconn, nil}
	for !connClosed {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		rbuf.Grow(8192)
		rbuf.ReadFrom(reader)
		if nil != reader.Err {
			conn.Close()
			connClosed = true
		}
		ress, err := remote.HandleRequestBuffer(&rbuf, ctx)
		if nil != err {
			if err != event.EBNR {
				log.Printf("[ERROR]connection %s:%d error:%v", ctx.User, ctx.ConnIndex, err)
				conn.Close()
				connClosed = true
				return
			}
		} else {
			writeEvents(ress, &wbuf)
			if !writeTaskRunning && len(ctx.User) > 0 && ctx.ConnIndex >= 0 {
				writeTaskRunning = true
				go func() {
					var lastEventTime time.Time
					queue := remote.GetEventQueue(ctx.ConnId, true)
					for !connClosed {
						evs, err := queue.PeekMulti(10, 1*time.Millisecond, false)
						now := time.Now()
						if ctx.Closing {
							evs = []event.Event{&event.ChannelCloseACKEvent{}}
						} else {
							if nil != err {
								if remote.GetSessionTableSize() > 0 && lastEventTime.Add(10*time.Second).Before(now) {
									evs = []event.Event{event.NewHeartBeatEvent()}
								} else {
									continue
								}
							}
						}

						err = writeEvents(evs, &wbuf)
						if ctx.Closing {
							break
						}
						lastEventTime = now
						if nil != err {
							log.Printf("TCP write error:%v", err)
							conn.Close()
							break
						} else {
							queue.DiscardPeeks(false)
						}
					}
					remote.ReleaseEventQueue(queue)
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
func dumpServerSession(args []string, c io.Writer) error {
	remote.DumpAllSession(c)
	return nil
}
func dumpServerQueue(args []string, c io.Writer) error {
	remote.DumpAllQueue(c)
	return nil
}

func main() {
	ots.RegisterHandler("vstat", dumpServerStat, 0, 0, "VStat                                 Dump server stat")
	ots.RegisterHandler("sls", dumpServerSession, 0, 0, "SLS                                  List server sessions")
	ots.RegisterHandler("qls", dumpServerQueue, 0, 0, "QLS                                  List server event queues")
	err := ots.StartTroubleShootingServer(remote.ServerConf.AdminListen)
	if nil != err {
		log.Printf("Failed to start admin server with reason:%v", err)
		return
	}
	startLocalProxyServer(remote.ServerConf.Listen)
}
