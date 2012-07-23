package paas

import (
	"bufio"
	"log"
	"net"
	"strings"
)

func HandleConn(sessionId int32, conn net.Conn) {
	bufreader := bufio.NewReader(conn)
	b, err := bufreader.Peek(7)
	if nil != err {
		log.Printf("Failed to peek data:%s\n", err.Error())
		conn.Close()
		return
	}
	session := newSessionConnection(sessionId, conn, bufreader)
	//log.Printf("First str:%s\n", string(b))
	if strings.EqualFold(string(b), "Connect") {
		session.Type = HTTPS_TUNNEL
	} else if b[0] == byte(4) || b[0] == byte(5) {
		session.Type = SOCKS_TUNNEL
	} else {
		session.Type = HTTP_TUNNEL
	}
	for session.State != STATE_SESSION_CLOSE {
		err := session.process()
		if nil != err {
			return
		}
	}
	if nil != session.RemoteConn {
		session.RemoteConn.GetConnectionManager().RecycleRemoteConnection(session.RemoteConn)
	}
}
