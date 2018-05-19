package channel

import (
	"io"
	"sync"

	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
)

var p2spSessionTable = make(map[string]map[string]map[mux.MuxSession]bool)
var p2spSessionMutex sync.Mutex

func addP2spSession(roomID string, cid string, session mux.MuxSession) bool {
	p2spSessionMutex.Lock()
	defer p2spSessionMutex.Unlock()
	m1, exist := p2spSessionTable[roomID]
	if !exist {
		m1 = make(map[string]map[mux.MuxSession]bool)
		p2spSessionTable[roomID] = m1
	}

	sessions, exist := m1[cid]
	if !exist {
		if len(m1) >= 2 {
			logger.Error("Already TWO users joined for P2SPId:%s", roomID)
			return false
		}
		sessions = make(map[mux.MuxSession]bool)
		m1[cid] = sessions
		logger.Info("P2SP Room:%s have %d members, '%s' just joined.", roomID, len(m1), cid)
	}
	sessions[session] = true
	return true
}

func removeP2spSession(roomID string, cid string, session mux.MuxSession) bool {
	p2spSessionMutex.Lock()
	defer p2spSessionMutex.Unlock()
	m1, exist := p2spSessionTable[roomID]
	if !exist {
		return false
	}
	sessions, exist := m1[cid]
	if !exist {
		return false
	}
	delete(sessions, session)
	if len(sessions) == 0 {
		delete(m1, cid)
		logger.Info("P2SP Room:%s have %d members, '%s' just exit.", roomID, len(m1), cid)
		if len(m1) == 0 {
			delete(p2spSessionTable, roomID)
		}
	}
	return true
}

func openPeerStream(roomID string, cid string) (mux.MuxStream, bool) {
	p2spSessionMutex.Lock()
	defer p2spSessionMutex.Unlock()
	m1, exist := p2spSessionTable[roomID]
	if !exist {
		logger.Error("No P2SP Room found for %s", roomID)
		return nil, false
	}
	for connID, sessions := range m1 {
		if connID != cid {
			for session := range sessions {
				stream, err := session.OpenStream()
				if nil == err {
					logger.Debug("Create peer stream %s:(%s <-> %s)", roomID, cid, connID)
					return stream, true
				}
				logger.Error("Failed to create peer P2SP stream with %s:%s", roomID, connID)
				return nil, false
			}
		}
	}
	logger.Error("No P2SP Peer found for %s:%s", roomID, cid)
	return nil, false
}

func handleP2spProxyStream(stream mux.MuxStream, ctx *sessionContext) {
	peerStream, success := openPeerStream(ctx.auth.P2SPRoomId, ctx.auth.P2SPConnId)
	if !success {
		stream.Close()
		return
	}
	closeSig := make(chan bool, 1)
	go func() {
		io.Copy(stream, peerStream)
		closeSig <- true
		stream.Close()
	}()
	io.Copy(peerStream, stream)
	<-closeSig
	stream.Close()
	peerStream.Close()
}
