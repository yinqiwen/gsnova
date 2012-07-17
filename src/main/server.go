package main

import (
	"log"
	"net"
	"paas"
	"sync/atomic"
	//"runtime"
)

const (
	MAX_READ_CHUNK_SIZE = 8192
)

var seed int32 = 0

func handleConn(conn *net.TCPConn) {
	sessionID := atomic.AddInt32(&seed, 1)
	//log.Printf("Session:%d created\n", sessionID)
	paas.HandleConn(sessionID, conn)
}

func handleServer(lp *net.TCPListener) {
	for {
		conn, err := lp.AcceptTCP()
		if nil != err {
			continue
		}
		//runtime.GC()
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
		log.Fatalf("Can NOT listen on address:%s\n", addr)
		return false
	}
	log.Printf("Listen on address:%s\n", addr)
	handleServer(lp)
	return true
}
