package channel

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
)

type p2pNode struct {
	sessions    map[mux.MuxSession]bool
	peerPubAddr string
	peerPriAddr string
}

var p2pSessionTable = make(map[string]map[string]*p2pNode)
var p2pSessionMutex sync.Mutex

func addP2PSession(auth *mux.AuthRequest, session mux.MuxSession, raddr net.Addr) bool {
	p2pSessionMutex.Lock()
	defer p2pSessionMutex.Unlock()
	m1, exist := p2pSessionTable[auth.P2PToken]
	if !exist {
		m1 = make(map[string]*p2pNode)
		p2pSessionTable[auth.P2PToken] = m1
	}

	sessions, exist := m1[auth.P2PConnID]
	if !exist {
		if len(m1) >= 2 {
			logger.Error("Already TWO users joined for P2P Room:%s", auth.P2PToken)
			return false
		}
		sessions = &p2pNode{
			sessions: make(map[mux.MuxSession]bool),
		}
		m1[auth.P2PConnID] = sessions
		logger.Info("P2P Room:%s have %d members, '%s' just joined.", auth.P2PToken, len(m1), auth.P2PConnID)
	}
	if len(auth.P2PPriAddr) > 0 {
		logger.Info("Recv P2P Room:%s tunnel conn at from %v with req:%v", auth.P2PToken, raddr, auth)
		sessions.peerPriAddr = auth.P2PPriAddr
		if len(auth.P2PPubAddr) == 0 {
			sessions.peerPubAddr = raddr.String()
		} else {
			sessions.peerPubAddr = auth.P2PPubAddr
		}
	} else {
		sessions.sessions[session] = true
	}
	return true
}

func removeP2PSession(auth *mux.AuthRequest, session mux.MuxSession) bool {
	if nil == auth {
		return false
	}

	p2pSessionMutex.Lock()
	defer p2pSessionMutex.Unlock()
	m1, exist := p2pSessionTable[auth.P2PToken]
	if !exist {
		return false
	}
	sessions, exist := m1[auth.P2PConnID]
	if !exist {
		return false
	}
	delete(sessions.sessions, session)
	if len(sessions.sessions) == 0 {
		delete(m1, auth.P2PConnID)
		logger.Info("P2P Room:%s have %d members, '%s' just exit.", auth.P2PToken, len(m1), auth.P2PConnID)
		if len(m1) == 0 {
			delete(p2pSessionTable, auth.P2PToken)
		}
	}
	if len(auth.P2PPriAddr) > 0 {
		sessions.peerPriAddr = ""
		sessions.peerPubAddr = ""
	}
	return true
}

func getPeerAddr(auth *mux.AuthRequest) (string, string) {
	p2pSessionMutex.Lock()
	defer p2pSessionMutex.Unlock()
	m1, exist := p2pSessionTable[auth.P2PToken]
	if !exist {
		logger.Error("No P2P Room found for %s", auth.P2PToken)
		return "", ""
	}
	for connID, sessions := range m1 {
		if connID != auth.P2PConnID {
			return sessions.peerPriAddr, sessions.peerPubAddr
		}
	}
	return "", ""
}

func openPeerStream(roomID string, cid string) (mux.MuxStream, bool) {
	p2pSessionMutex.Lock()
	defer p2pSessionMutex.Unlock()
	m1, exist := p2pSessionTable[roomID]
	if !exist {
		logger.Error("No P2P Room found for %s", roomID)
		return nil, false
	}
	for connID, sessions := range m1 {
		if connID != cid {
			for session := range sessions.sessions {
				stream, err := session.OpenStream()
				if nil == err {
					logger.Debug("Create peer stream %s:(%s <-> %s)", roomID, cid, connID)
					return stream, true
				}
				logger.Error("Failed to create peer P2P stream with %s:%s", roomID, connID)
				return nil, false
			}
		}
	}
	logger.Error("No P2P Peer found for %s:%s", roomID, cid)
	return nil, false
}

func handleP2PProxyStream(stream mux.MuxStream, ctx *sessionContext) {
	peerStream, success := openPeerStream(ctx.auth.P2PToken, ctx.auth.P2PConnID)
	if !success {
		stream.Close()
		return
	}
	start := time.Now()
	closeSig := make(chan bool, 1)
	go func() {
		io.Copy(stream, peerStream)
		logger.Info("P2P:Cost %v to copy local to remote", time.Now().Sub(start))
		stream.Close()
		closeSig <- true
	}()
	io.Copy(peerStream, stream)
	logger.Info("P2P:Cost %v to copy remote to local", time.Now().Sub(start))
	<-closeSig
	stream.Close()
	peerStream.Close()

}
