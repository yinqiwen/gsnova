package proxy

import (
	"event"
	"log"
	"net"
	"net/http/httputil"
	"strings"
	"time"
	"util"
)

func (gae *GAEHttpConnection) tunnel_write(conn *SessionConnection) {
	for !gae.closed {
		select {
		case ev := <-gae.tunnelChannel:
			err, res := gae.requestEvent(gae.client, conn, ev)
			if nil == err {
				gae.handleTunnelResponse(conn, res)
			}
		}
	}
}

func (gae *GAEHttpConnection) tunnel_read(conn *SessionConnection) {
	wait := 1 * time.Second
	for !gae.closed {
		read := &event.SocketReadEvent{Timeout: 25, MaxRead: 256 * 1024}
		read.SetHash(conn.SessionID)
		err, res := gae.requestEvent(gae.client, conn, read)
		if nil == err {
			wait = 1 * time.Second
			gae.handleTunnelResponse(conn, res)
		} else {
			time.Sleep(wait)
			wait = 2 * wait
		}
	}
}

func (gae *GAEHttpConnection) doCloseTunnel() {
	if nil != gae.tunnelChannel {
		closeEv := &event.SocketConnectionEvent{}
		closeEv.Status = event.TCP_CONN_CLOSED
		closeEv.SetHash(gae.sess.SessionID)
		gae.tunnelChannel <- closeEv
	}
}

func (gae *GAEHttpConnection) handleTunnelResponse(conn *SessionConnection, ev event.Event) error {
	switch ev.GetType() {
	case event.EVENT_TCP_CONNECTION_TYPE:
		cev := ev.(*event.SocketConnectionEvent)
		if cev.Status == event.TCP_CONN_CLOSED {
			if gae.tunnel_remote_addr == cev.Addr {
				conn.Close()
			}
		}
	case event.EVENT_TCP_CHUNK_TYPE:
		chunk := ev.(*event.TCPChunkEvent)
		_, err := conn.LocalRawConn.Write(chunk.Content)
		if nil != err {
			log.Printf("[%d]Failed to write  data to local client:%v.\n", ev.GetHash(), err)
			conn.Close()
		}
	default:
		log.Printf("Unexpected event type:%d\n", ev.GetType())
	}
	return nil
}

func (gae *GAEHttpConnection) requestOverTunnel(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	if nil == gae.tunnelChannel {
		gae.tunnelChannel = make(chan event.Event)
		go gae.tunnel_write(conn)
		go gae.tunnel_read(conn)
	}
	switch ev.GetType() {
	case event.HTTP_REQUEST_EVENT_TYPE:
		req := ev.(*event.HTTPRequestEvent)
		default_port := "80"
		if strings.EqualFold(req.RawReq.Method, "CONNECT") {
			conn.State = STATE_RECV_HTTP_CHUNK
			default_port = "443"
		} else {
			conn.State = STATE_RECV_HTTP
		}
		log.Printf("Session[%d]Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
		scd := &event.SocketConnectWithDataEvent{}
		scd.Content, err = httputil.DumpRequest(req.RawReq, true)
		if nil != err {
			log.Printf("Session[%d]Failed to encode request to bytes", req.GetHash())
			return
		}
		scd.SetHash(ev.GetHash())
		scd.Net = "tcp"
		scd.Addr = req.RawReq.Host
		if !strings.Contains(scd.Addr, ":") {
			scd.Addr = net.JoinHostPort(req.RawReq.Host, default_port)
		}
		gae.tunnel_remote_addr = scd.Addr
		gae.tunnelChannel <- scd
	case event.HTTP_CHUNK_EVENT_TYPE:
		chunk := ev.(*event.HTTPChunkEvent)
		tcp_chunk := &event.TCPChunkEvent{Content: chunk.Content}
		tcp_chunk.SetHash(ev.GetHash())
		gae.tunnelChannel <- tcp_chunk
		conn.State = STATE_RECV_HTTP_CHUNK
	}
	return nil, nil
}
