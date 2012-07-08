package paas

import (
	//	"event"
	"net"
	"strings"
	//"net/http"
	"bufio"
)

//const (
//	HTTP_TUNNEL           = 1
//	HTTPS_TUNNEL          = 2
//	TAKE_OVER             = 3
//	FULL_REQUEST_EXPECTED = 4
//)

//var gae GAEEventService

func dispatchSocksConnection(sessionId int32, r *bufio.Reader, conn net.Conn) {

}

func dispatchHttpsConnection(sessionId int32, r *bufio.Reader, conn net.Conn) {
}

func dispatchHttpConnection(sessionId int32, r *bufio.Reader, conn net.Conn) {

}

func DispatchConn(sessionId int32, conn net.Conn) {
	bufreader := bufio.NewReader(conn)
	b, err := bufreader.Peek(6)
	if nil != err{
	   return
	}
	if strings.EqualFold(string(b), "Connect") {
		dispatchHttpsConnection(sessionId, bufreader, conn)
	} else if b[0] == byte(4) || b[0] == byte(5) {
		dispatchSocksConnection(sessionId, bufreader, conn)
	} else {
		dispatchHttpConnection(sessionId, bufreader, conn)
	}
}
