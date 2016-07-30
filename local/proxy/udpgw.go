package proxy

import (
	"io"
	"log"
	"net"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
)

type udpConn struct {
	id      uint32
	srcip   int64
	srcport uint16
	dstip   int64
	dstport uint16
}

var udpConnTable = make(map[uint32]*udpConn)

func handleUDPGatewayConn(conn net.Conn, p Proxy) {
	queue := event.NewEventQueue()
	connClosed := false
	go func() {
		for !connClosed {
			ev, err := queue.Read(1 * time.Second)
			if err != nil {
				if err != io.EOF {
					continue
				}
				return
			}
			//log.Printf("Session:%d recv event:%T", sid, ev)
			switch ev.(type) {
			case *event.UDPEvent:
				//donothing now
			default:
				log.Printf("Invalid event type:%T to process", ev)
			}
		}
	}()
	//bufconn := bufio.NewReader(conn)
	for {
		//read udpgw header(3byte)
	}
}
