package paas

import (
	"net"
	"strings"
	//"net/http"
	"bufio"
)

func HandleConn(sessionId int32, conn net.Conn) {
	bufreader := bufio.NewReader(conn)
	b, err := bufreader.Peek(6)
	if nil != err {
		return
	}
	session := newSessionConnection(sessionId, conn, bufreader)
	if strings.EqualFold(string(b), "Connect") {
		session.Type = HTTPS_TUNNEL
	} else if b[0] == byte(4) || b[0] == byte(5) {
		session.Type = SOCKS_TUNNEL
	} else {
		session.Type = HTTP_TUNNEL
	}
	for ;session.State != STATE_SESSION_CLOSE;{
	    err := session.process()
	    if nil != err{
	       return
	    }
	}
}
