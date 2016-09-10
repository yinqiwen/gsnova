package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/remote"
)

var totalConn int32

type vpsServer struct {
	port       uint32
	aliveConns int32
	createTime time.Time
	retireTime time.Time
	lp         net.Listener
}

var activeDynamicServers = make(map[*vpsServer]bool)
var retiredDynamicServers = make(map[*vpsServer]bool)
var dynamicServerMutex sync.Mutex

func activeDynamicServerSize() int {
	dynamicServerMutex.Lock()
	defer dynamicServerMutex.Unlock()
	return len(activeDynamicServers)
}
func retiredDynamicServerSize() int {
	dynamicServerMutex.Lock()
	defer dynamicServerMutex.Unlock()
	return len(retiredDynamicServers)
}

func selectDynamicVPServer() *vpsServer {
	var vs *vpsServer
	if len(remote.ServerConf.CandidateDynamicPort) > 0 && remote.ServerConf.MaxDynamicPort > 0 {
		remote.ServerConf.MaxDynamicPort = len(remote.ServerConf.CandidateDynamicPort)
	}
	dynamicServerMutex.Lock()
	if len(activeDynamicServers) >= remote.ServerConf.MaxDynamicPort {
		now := time.Now()
		lifeCycle := remote.ServerConf.DynamicPortLifeCycle
		if lifeCycle == 0 {
			lifeCycle = 1800
		}
		for vss := range activeDynamicServers {
			if len(remote.ServerConf.CandidateDynamicPort) == 0 && vss.createTime.Add(time.Duration(lifeCycle)*time.Second).Before(now) {
				vss.retireTime = now
				retiredDynamicServers[vss] = true
				delete(activeDynamicServers, vss)
			} else {
				if nil == vs || vss.aliveConns < vs.aliveConns {
					vs = vss
				}
			}
		}
	}
	if nil == vs {
		vs = new(vpsServer)
		var err error
		dynAddr := ":0"
		if len(remote.ServerConf.CandidateDynamicPort) > 0 {
			for _, port := range remote.ServerConf.CandidateDynamicPort {
				dynAddr = fmt.Sprintf(":%d", port)
				err = startDynamicServer(dynAddr, vs)
				if nil == err {
					break
				}
			}
		} else {
			err = startDynamicServer(dynAddr, vs)

		}
		if nil == err {
			activeDynamicServers[vs] = true
		} else {
			vs = nil
		}
	}
	clearExpiredDynamicVPServer()
	dynamicServerMutex.Unlock()
	return vs
}

func clearExpiredDynamicVPServer() {
	now := time.Now()
	for vs := range retiredDynamicServers {
		if vs.aliveConns == 0 && vs.retireTime.Add(10*time.Second).Before(now) {
			log.Printf("Close dynamic listen server :%d", vs.port)
			vs.lp.Close()
			delete(retiredDynamicServers, vs)
		}
	}
}

func serveProxyConn(conn net.Conn, vs *vpsServer) {
	if nil != vs {
		atomic.AddInt32(&vs.aliveConns, 1)
	}
	atomic.AddInt32(&totalConn, 1)
	bufconn := bufio.NewReader(conn)

	deferFunc := func() {
		conn.Close()
		atomic.AddInt32(&totalConn, -1)
		if nil != vs {
			atomic.AddInt32(&vs.aliveConns, -1)
		}
	}
	defer deferFunc()

	ctx := remote.NewConnContext()
	writeEvents := func(evs []event.Event, buf *bytes.Buffer) error {
		if len(evs) > 0 {
			buf.Reset()
			for _, ev := range evs {
				if nil != ev {
					event.EncryptEvent(buf, ev, &ctx.CryptoContext)
				}
			}
			if buf.Len() > 0 {
				conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
				b := buf.Bytes()
				_, err := conn.Write(b)

				return err
			}
		}
		return nil
	}

	var rbuf bytes.Buffer
	var wbuf bytes.Buffer

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
							if remote.ServerConf.MaxDynamicPort > 0 {
								nvs := selectDynamicVPServer()
								if nil != nvs {
									evs = append(evs, &event.PortUnicastEvent{Port: nvs.port})
								}
							}
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
						if nil != err {
							log.Printf("TCP write error####:%v %d", err, len(evs))
						} else {
							queue.DiscardPeeks(false)
						}
						if ctx.Closing {
							break
						}
						lastEventTime = now
						if nil != err {
							log.Printf("TCP write error:%v", err)
							conn.Close()
							break
						} else {
							//queue.DiscardPeeks(false)
						}
					}
					remote.ReleaseEventQueue(queue)
				}()
			}
		}
	}
}
func listenTCPServer(addr string, vs *vpsServer) (net.Listener, error) {
	if len(addr) == 0 {
		log.Fatalf("Empty listen address.")
		return nil, nil
	}
	//var lp *net.TCPListener
	lp, err := net.Listen("tcp", addr)
	if nil != err {
		log.Printf("Can NOT listen on address:%s", addr)
		return nil, err
	}
	tcpaddr := lp.Addr().(*net.TCPAddr)
	log.Printf("Listen on address %v", tcpaddr)
	if nil != vs {
		vs.port = uint32(tcpaddr.Port)
		vs.lp = lp
		vs.createTime = time.Now()
	}
	return lp, nil
}

func startDynamicServer(addr string, vs *vpsServer) error {
	lp, err := listenTCPServer(addr, vs)
	if nil != err {
		return err
	}
	go func() {
		for {
			conn, err := lp.Accept()
			if nil != err {
				log.Printf("Accept %s error:%v", addr, err)
				return
			}
			go serveProxyConn(conn, vs)
		}
	}()
	return nil
}

func startLocalProxyServer(addr string) error {
	lp, err := listenTCPServer(addr, nil)
	if nil != err {
		return err
	}
	for {
		conn, err := lp.Accept()
		if nil != err {
			continue
		}
		go serveProxyConn(conn, nil)
	}
	return nil
}
