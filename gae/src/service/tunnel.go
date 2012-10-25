package service

import (
	"appengine"
	"appengine/socket"
	"event"
	//"fmt"
	//"net/http"
	//	"strconv"
	"sync"
	"time"
)

type tunnelConn struct {
	Net  string
	Addr string
	c    *socket.Conn
}

var tunnel_conn_map = make(map[string]map[uint32]*tunnelConn)
var tunnel_conn_map_mutex sync.Mutex

func closeTunnelConn(uid string, sid uint32) {
	tunnel_conn_map_mutex.Lock()
	defer tunnel_conn_map_mutex.Unlock()
	if m, exist := tunnel_conn_map[uid]; exist {
		if v, ok := m[sid]; ok {
			v.c.Close()
			delete(m, sid)
		}
	} else {
		tunnel_conn_map[uid] = make(map[uint32]*tunnelConn)
	}
}

func getTunnelConn(uid string, sid uint32) (*tunnelConn, bool) {
	tunnel_conn_map_mutex.Lock()
	defer tunnel_conn_map_mutex.Unlock()
	if m, exist := tunnel_conn_map[uid]; exist {
		v, ok := m[sid]
		return v, ok
	} else {
		tunnel_conn_map[uid] = make(map[uint32]*tunnelConn)
	}
	return nil, false
}

func saveTunnelConn(uid string, sid uint32, c *tunnelConn) {
	tunnel_conn_map_mutex.Lock()
	defer tunnel_conn_map_mutex.Unlock()
	tunnel_conn_map[uid][sid] = c
}

func getCreateTunnelConn(context appengine.Context, uid string, ev *event.SocketConnectWithDataEvent) (*tunnelConn, bool, error) {
	created := false
	if c, exist := getTunnelConn(uid, ev.GetHash()); exist {
		if c.Addr == ev.Addr {
			return c, created, nil
		} else {
			closeTunnelConn(uid, ev.GetHash())
		}
	}
	c := &tunnelConn{Net: ev.Net, Addr: ev.Addr}
	conn, err := socket.DialTimeout(context, ev.Net, ev.Addr, time.Duration(ev.Timeout)*time.Second)
	if nil == err {
		c.c = conn
		created = true
		saveTunnelConn(uid, ev.GetHash(), c)
		return c, created, nil
	}
	context.Errorf("[%d]Failed to connect %s for reason:%v", ev.GetHash(), ev.Addr, err)
	return nil, created, err
}

func ganerateCloseEvent(sid uint32) *event.SocketConnectionEvent {
	close_ev := &event.SocketConnectionEvent{}
	close_ev.SetHash(sid)
	close_ev.Status = event.TCP_CONN_CLOSED
	return close_ev
}

func TunnelConn(context appengine.Context, tags *event.EventHeaderTags, ev *event.SocketConnectWithDataEvent) event.Event {
	close_ev := ganerateCloseEvent(ev.GetHash())
	close_ev.Addr = ev.Addr
	if c, created, err := getCreateTunnelConn(context, tags.UserToken, ev); nil == err {
		if len(ev.Content) > 0 {
			_, err = c.c.Write(ev.Content)
			if nil != err {
				context.Errorf("[%d]Failed totunnel write initial data for reason:%v", ev.GetHash(), err)
				return close_ev
			}
		}
		if created {
			connected_ev := close_ev
			connected_ev.Status = event.TCP_CONN_OPENED
			return connected_ev
		} else {
			return nil
		}
	}
	return close_ev
}

func TunnelWrite(context appengine.Context, tags *event.EventHeaderTags, ev *event.TCPChunkEvent) event.Event {
	close_ev := ganerateCloseEvent(ev.GetHash())
	if c, exist := getTunnelConn(tags.UserToken, ev.GetHash()); exist {
		_, err := c.c.Write(ev.Content)
		if nil == err {
			return nil
		}
		close_ev.Addr = c.Addr
		closeTunnelConn(tags.UserToken, ev.GetHash())
		context.Errorf("[%d]Failed to tunnel write for reason:%v", ev.GetHash(), err)
	} else {
		context.Errorf("Failed to find conn for %s:%d", tags.UserToken, ev.GetHash())
	}
	return close_ev
}

func TunnelRead(context appengine.Context, tags *event.EventHeaderTags, ev *event.SocketReadEvent) event.Event {
	close_ev := ganerateCloseEvent(ev.GetHash())
	if c, exist := getTunnelConn(tags.UserToken, ev.GetHash()); exist {
		buf := make([]byte, ev.MaxRead)
		c.c.SetReadDeadline(time.Now().Add(time.Duration(ev.Timeout) * time.Second))
		_, err := c.c.Read(buf)
		if nil == err {
			return nil
		}
		close_ev.Addr = c.Addr
		closeTunnelConn(tags.UserToken, ev.GetHash())
		context.Errorf("[%d]Failed to tunnel read for reason:%v", ev.GetHash(), err)
	} else {
		context.Errorf("Failed to find conn for %s:%d", tags.UserToken, ev.GetHash())
	}
	return close_ev
}

func TunnelSocketConnection(context appengine.Context, tags *event.EventHeaderTags, ev *event.SocketConnectionEvent) event.Event {
	if ev.Status == event.TCP_CONN_CLOSED {
		closeTunnelConn(tags.UserToken, ev.GetHash())
	}
	return nil
}
