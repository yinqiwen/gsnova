package channel

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
)

var p2pConnID string

func init() {
	p2pConnID = helper.RandAsciiString(32)
}

type DeadLineAccetor interface {
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

type muxSessionHolder struct {
	creatTime       time.Time
	expireTime      time.Time
	activeTime      time.Time
	muxSession      mux.MuxSession
	retiredSessions map[mux.MuxSession]bool
	server          string
	Channel         LocalChannel
	sessionMutex    sync.Mutex
	conf            *ProxyChannelConfig
	heatbeating     bool
}

func (s *muxSessionHolder) tryCloseRetiredSessions() {
	for retiredSession := range s.retiredSessions {
		if retiredSession.NumStreams() <= 0 {
			logger.Debug("Close retired mux session since it's has no active stream.")
			retiredSession.Close()
			delete(s.retiredSessions, retiredSession)
		}
	}
}

func (s *muxSessionHolder) dumpStat(w io.Writer) {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()
	s.tryCloseRetiredSessions()
	fmt.Fprintf(w, "Server:%s, CreateTime:%v, RetireTime:%v, RetireSessionNum:%v\n", s.server, s.creatTime.Format("15:04:05"), s.expireTime.Format("15:04:05"), len(s.retiredSessions))
}

func (s *muxSessionHolder) close() {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	if nil != s.muxSession {
		s.muxSession.Close()
		s.muxSession = nil
	}
}
func (s *muxSessionHolder) check() {
	if nil != s.muxSession && !s.expireTime.IsZero() && s.expireTime.Before(time.Now()) {
		s.retiredSessions[s.muxSession] = true
		s.muxSession = nil
	}
}

func (s *muxSessionHolder) getNewStream() (mux.MuxStream, error) {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()
	defer func() {
		s.tryCloseRetiredSessions()
	}()
	s.check()
	if nil == s.muxSession {
		s.init(false)
	}
	if nil == s.muxSession {
		return nil, pmux.ErrSessionShutdown
	}
	s.activeTime = time.Now()
	return s.muxSession.OpenStream()
}

func (s *muxSessionHolder) heartbeat(interval int) {
	if s.heatbeating {
		return
	}
	for {
		s.heatbeating = true
		select {
		case <-time.After(time.Duration(interval) * time.Second):
			s.sessionMutex.Lock()
			s.check()
			session := s.muxSession
			s.sessionMutex.Unlock()
			if nil != session {
				if s.Channel.Features().Pingable {
					_, err := session.Ping()
					if err != nil {
						logger.Error("[ERR]: Ping remote:%s failed: %v", s.server, err)
						s.close()
					} else {
						// if duration > time.Duration(100)*time.Millisecond {
						// 	logger.Debug("Cost %v to ping remote:%s", duration, s.server)
						// }
					}
				}
			} else {
				if !s.conf.lazyConnect && time.Now().Sub(s.activeTime) > time.Duration(s.conf.HibernateAfterSecs)*time.Second {
					s.init(true)
				}
			}
		}
	}
}

func (s *muxSessionHolder) init(lock bool) error {
	if lock {
		s.sessionMutex.Lock()
		defer s.sessionMutex.Unlock()
	}
	if nil != s.muxSession {
		return nil
	}
	session, err := s.Channel.CreateMuxSession(s.server, s.conf)
	if nil == err && nil != session {
		cipherMethod := s.conf.Cipher.Method
		if strings.HasPrefix(s.server, "https://") || strings.HasPrefix(s.server, "wss://") || strings.HasPrefix(s.server, "tls://") || strings.HasPrefix(s.server, "quic://") || strings.HasPrefix(s.server, "http2://") {
			cipherMethod = "none"
		}
		err, authReq, _ := clientAuthMuxSession(session, cipherMethod, s.conf, "", "", true, false)
		if nil != err {

			return err
		}

		s.creatTime = time.Now()
		s.muxSession = session
		features := s.Channel.Features()
		if features.AutoExpire {
			expireAfter := 1800
			if s.conf.ReconnectPeriod > 0 {
				expireAfter = helper.RandBetween(s.conf.ReconnectPeriod-s.conf.RCPRandomAdjustment, s.conf.ReconnectPeriod+s.conf.RCPRandomAdjustment)
			}
			logger.Debug("Mux session woulde expired after %d seconds.", expireAfter)
			s.expireTime = time.Now().Add(time.Duration(expireAfter) * time.Second)
		}

		if features.Pingable && s.conf.HeartBeatPeriod > 0 {
			go s.heartbeat(s.conf.HeartBeatPeriod)
		}
		if DirectChannelName != s.conf.Name {
			if len(defaultProxyLimitConfig.BlackList) > 0 || len(defaultProxyLimitConfig.WhiteList) > 0 {
				go ServProxyMuxSession(session, authReq, nil)
			}
		}

		return nil
	}
	if nil == err {
		err = fmt.Errorf("Empty error to create session")
	}
	return err
}

var localChannelTable = make(map[string]*LocalProxyChannel)
var localChannelMutex sync.Mutex

type LocalProxyChannel struct {
	Conf ProxyChannelConfig
	//sessions       map[*muxSessionHolder]bool
	sessions       []*muxSessionHolder
	cursor         int32
	lastActiveTime time.Time
	autoExpire     bool
	p2pSessions    sync.Map
}

func (ch *LocalProxyChannel) isP2PSessionEstablisehd() bool {
	empty := true
	ch.p2pSessions.Range(func(key, value interface{}) bool {
		empty = false
		return false
	})
	return !empty
}

func (ch *LocalProxyChannel) closeAll() {
	for _, holder := range ch.sessions {
		if nil != holder {
			holder.close()
		}
	}
}

func (ch *LocalProxyChannel) setP2PSession(c net.Conn, s mux.MuxSession, authReq *mux.AuthRequest) {
	ch.p2pSessions.Store(s, true)
	if nil != s {
		go func() {
			failCount := 0
			for failCount < 3 {
				time.Sleep(3 * time.Second)
				if duration, err := s.Ping(); nil != err {
					logger.Error("P2P Tunnl Ping error:%v after %v", err, duration)
					failCount++
					time.Sleep(3 * time.Second)
					continue
				}
				failCount = 0
			}
			ch.p2pSessions.Delete(s)
		}()
		if len(defaultProxyLimitConfig.BlackList) > 0 || len(defaultProxyLimitConfig.WhiteList) > 0 {
			go ServProxyMuxSession(s, authReq, nil)
		}
	}
}

func (ch *LocalProxyChannel) createMuxSessionByProxy(p LocalChannel, server string, init bool) (*muxSessionHolder, error) {
	holder := &muxSessionHolder{
		conf:            &ch.Conf,
		Channel:         p,
		server:          server,
		retiredSessions: make(map[mux.MuxSession]bool),
	}
	var err error
	if init {
		err = holder.init(true)
	}

	if nil == err {
		ch.sessions = append(ch.sessions, holder)
		//ch.sessions[holder] = true
		return holder, nil
	}
	return nil, err
}

func (ch *LocalProxyChannel) getMuxStream() (stream mux.MuxStream, err error) {
	ch.p2pSessions.Range(func(key, value interface{}) bool {
		session := key.(mux.MuxSession)
		stream, err = session.OpenStream()
		if nil == err {
			return false
		}
		return true
	})
	if nil != stream {
		return
	}

	c := atomic.LoadInt32(&ch.cursor)
	for i := 0; i < len(ch.sessions); i++ {
		if int(c) >= len(ch.sessions) {
			c = 0
		}
		holder := ch.sessions[c]
		stream, err = holder.getNewStream()
		if nil != err {
			if err == pmux.ErrSessionShutdown {
				holder.close()
			}
			logger.Debug("Try to get next session since current session failed to open new stream with err:%v", err)
			c++
		} else {
			if ch.autoExpire {
				ch.lastActiveTime = time.Now()
			}
			atomic.StoreInt32(&ch.cursor, c+1)
			return
		}
	}
	if nil == stream {
		err = fmt.Errorf("Create mux porxy stream failed")
	}
	return
}

func (ch *LocalProxyChannel) Init(lock bool) bool {
	conf := &ch.Conf
	success := false
	for _, server := range conf.ServerList {
		u, err := url.Parse(server)
		if nil != err {
			logger.Error("Invalid server url:%s with reason:%v", server, err)
			continue
		}

		schema := strings.ToLower(u.Scheme)
		if t, ok := LocalChannelTypeTable[schema]; !ok {
			logger.Error("[ERROR]No registe proxy for schema:%s", schema)
			continue
		} else {
			v := reflect.New(t)
			p := v.Interface().(LocalChannel)
			shouldInit := false
			if ch.Conf.P2S2PEnable && len(conf.P2PToken) > 0 {
				ch.Conf.lazyConnect = false
				shouldInit = true
				if ch.Conf.HeartBeatPeriod <= 0 || ch.Conf.HeartBeatPeriod >= 10 {
					ch.Conf.HeartBeatPeriod = 3
				}
			}
			for i := 0; i < conf.ConnsPerServer; i++ {
				if 0 == i {
					shouldInit = true
				}
				holder, err := ch.createMuxSessionByProxy(p, server, shouldInit)
				if nil != err {
					logger.Error("[ERROR]Failed to create mux session for %s:%d with reason:%v", server, i, err)
					break
				} else {
					success = true
					if len(conf.P2PToken) > 0 {
						if !conf.P2S2PEnable {
							holder.close()
						}
					}
				}
			}
			if success {
				if len(conf.P2PToken) > 0 {
					if !conf.P2S2PEnable {
						go startP2PSession(server, p, ch)
					}
				}
			}
		}
	}

	if success {
		logger.Notice("Proxy channel:%s init success", conf.Name)
		if lock {
			localChannelMutex.Lock()
			defer localChannelMutex.Unlock()
		}
		localChannelTable[conf.Name] = ch

	} else {
		logger.Error("[ERROR]Proxy channel:%s init failed", conf.Name)
	}
	return success
}

func DumpLoaclChannelStat(w io.Writer) {
	for _, pch := range localChannelTable {
		if pch.Conf.Name != DirectChannelName {
			for _, holder := range pch.sessions {
				if nil != holder {
					holder.dumpStat(w)
				}
			}
		}
	}
}

func NewProxyChannel(conf *ProxyChannelConfig) *LocalProxyChannel {
	channel := &LocalProxyChannel{
		Conf: *conf,
		//sessions: make(map[*muxSessionHolder]bool),
	}
	return channel
}

func GetMuxStreamByChannel(name string) (mux.MuxStream, *ProxyChannelConfig, error) {
	pch, exist := localChannelTable[name]
	if !exist {
		return nil, nil, fmt.Errorf("No proxy found to get mux session")
	}
	stream, err := pch.getMuxStream()
	return stream, &pch.Conf, err
}

func GetMuxStreamByURL(u *url.URL, defaultUser string, defaultCipher *CipherConfig) (mux.MuxStream, *ProxyChannelConfig, error) {
	key := u.String()
	localChannelMutex.Lock()
	defer localChannelMutex.Unlock()
	stream, conf, err := GetMuxStreamByChannel(key)
	if nil == err {
		return stream, conf, err
	}
	var cipher CipherConfig
	if nil != u.User {
		cipher.User = u.User.Username()
		cipher.Key, _ = u.User.Password()
		cipher.Method = u.Query().Get("method")
	}
	if len(cipher.User) == 0 {
		cipher.User = defaultUser
	}
	if len(cipher.Key) == 0 {
		cipher.Key = defaultCipher.Key
	}
	conf = &ProxyChannelConfig{
		Name:                u.String(),
		Enable:              true,
		ServerList:          []string{u.String()},
		Cipher:              cipher,
		ConnsPerServer:      3,
		HeartBeatPeriod:     30,
		ReconnectPeriod:     1800,
		RCPRandomAdjustment: 10,
		lazyConnect:         true,
	}
	conf.Adjust()
	ch := NewProxyChannel(conf)
	if ch.Init(false) {
		ch.autoExpire = true
		stream, err := ch.getMuxStream()
		expireLocalChannels()
		return stream, conf, err
	}
	return nil, nil, fmt.Errorf("Failed to init proxy channel by %s", key)
}

func StopLocalChannels() {
	for _, pch := range localChannelTable {
		for _, holder := range pch.sessions {
			if nil != holder && nil != holder.muxSession {
				holder.muxSession.Close()
			}
		}
	}
	localChannelTable = make(map[string]*LocalProxyChannel)
}

var expireTaskLauched int32

func expireLocalChannels() {
	if !atomic.CompareAndSwapInt32(&expireTaskLauched, 0, 1) {
		return
	}
	ticker := time.NewTicker(10 * time.Second)
	removeExpiredChannels := func() {

		localChannelMutex.Lock()
		defer localChannelMutex.Unlock()
		for key, ch := range localChannelTable {
			if !ch.autoExpire {
				continue
			}
			expire := true
			for _, session := range ch.sessions {
				session.check()
				session.tryCloseRetiredSessions()
				if len(session.retiredSessions) > 0 {
					expire = false
					break
				}
				if nil != session.muxSession && session.muxSession.NumStreams() > 0 {
					expire = false
					break
				}
				if nil != session.muxSession {
					session.close()
				}
			}
			if expire {
				logger.Info("Remove expired channel:%s", key)
				delete(localChannelTable, key)
			} else {

			}
		}
	}
	go func() {
		for {
			select {
			case <-ticker.C:
				removeExpiredChannels()
			}
		}
	}()
}
