package main

import (
//	"bufio"
//	"event"
//	"fmt"
//	"io"
	"net"
//	"net/http"
	"paas"
	"sync/atomic"
)

const (
	MAX_READ_CHUNK_SIZE = 8192
)

var seed int32 = 0

//func handleSessionConn(sessionID int32, conn net.Conn) {
//	req, err := http.ReadRequest(bufio.NewReader(conn))
//	if err != nil {
//		conn.Close()
//		return
//	}
	
//	var ev event.HTTPRequestEvent
//	ev.Url = req.RequestURI
//	ev.Method = req.Method
//	ch := make(chan event.Event)
//	for key, values := range req.Header {
//		for _, value := range values {
//			ev.AddHeader(key, value)
//		}
//	}
//	ev.AddHeader("Host", req.Host)
//	ev.SetHash(uint32(sessionID))
//}

func handleConn(conn *net.TCPConn) {
	sessionID := atomic.AddInt32(&seed, 1)
	paas.DispatchConn(sessionID, conn)
}

func handleServer(lp *net.TCPListener) {
	for {
		conn, err := lp.AcceptTCP()
		if nil != err {
			continue
		}
		go handleConn(conn)
	}
}

func startLocalProxyServer(addr string) bool {
	tcpaddr, err := net.ResolveTCPAddr("tcp", addr)
	if nil != err {
		return false
	}
	var lp *net.TCPListener
	lp, err = net.ListenTCP("tcp", tcpaddr)
	if nil != err {
		return false
	}
	handleServer(lp)
	return true
}

func main() {
    startLocalProxyServer("0.0.0.0:48188")
}
