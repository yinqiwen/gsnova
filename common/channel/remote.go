package channel

import (
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/ratelimit"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
)

var DefaultServerRateLimit RateLimitConfig
var rateLimitBuckets = make(map[string]*ratelimit.Bucket)
var rateLimitBucketLock sync.Mutex

var upBytesPool, downBytesPool *sync.Pool

type sessionContext struct {
	auth         *mux.AuthRequest
	activeIOTime time.Time
	streamCouter int32
	session      mux.MuxSession
	closed       bool
	isP2P        bool
	isP2PExahnge bool
}

func (ctx *sessionContext) close() {
	ctx.closed = true
	if ctx.isP2P && nil != ctx.auth {
		removeP2PSession(ctx.auth, ctx.session)
	}
	ctx.session.Close()
	emptySessions.Delete(ctx)
}

func getRateLimitBucket(user string) *ratelimit.Bucket {
	if nil == DefaultServerRateLimit.Limit {
		return nil
	}
	l, exist := DefaultServerRateLimit.Limit[user]
	if !exist {
		l, exist = DefaultServerRateLimit.Limit["*"]
		user = "*"
	}
	if !exist {
		return nil
	}
	limitPerSec := int64(-1)
	if len(l) > 0 {
		v, err := helper.ToBytes(l)
		if nil == err {
			limitPerSec = int64(v)
		}
	}
	if limitPerSec <= 0 {
		return nil
	}
	rateLimitBucketLock.Lock()
	defer rateLimitBucketLock.Unlock()
	r, ok := rateLimitBuckets[user]
	if !ok {
		r = ratelimit.NewBucket(1*time.Second, limitPerSec)
		rateLimitBuckets[user] = r
	}
	return r
}

var emptySessions sync.Map

func init() {
	upBytesPool = &sync.Pool{
		New: func() interface{} {
			blen := defaultMuxConfig.UpBufferSize
			if 0 == blen {
				blen = 16 * 1024
			}
			return make([]byte, blen)
		},
	}
	downBytesPool = &sync.Pool{
		New: func() interface{} {
			blen := defaultMuxConfig.DownBufferSize
			if 0 == blen {
				blen = 128 * 1024
			}
			return make([]byte, blen)
		},
	}
	go func() {
		if defaultMuxConfig.SessionIdleTimeout > 0 {
			sessionActiveTicker := time.NewTicker(10 * time.Second)
			for range sessionActiveTicker.C {
				emptySessions.Range(func(key, value interface{}) bool {
					ctx := key.(*sessionContext)
					ago := time.Now().Sub(ctx.activeIOTime)
					if ago > time.Duration(defaultMuxConfig.SessionIdleTimeout)*time.Second {
						ctx.close()
						logger.Error("Close mux session since it's not active since %v ago.", ago)
					}
					return true
				})
			}
		}
	}()
}

func isTimeoutErr(err error) bool {
	if err == pmux.ErrTimeout {
		return true
	}
	if err, ok := err.(net.Error); ok && err.Timeout() {
		return true
	}
	return false
}

func handleProxyStream(stream mux.MuxStream, ctx *sessionContext) {
	atomic.AddInt32(&ctx.streamCouter, 1)
	emptySessions.Delete(ctx)
	defer func() {
		if 0 == atomic.AddInt32(&ctx.streamCouter, -1) && !ctx.closed {
			emptySessions.Store(ctx, true)
		}
	}()
	creq, err := mux.ReadConnectRequest(stream)
	if nil != err {
		stream.Close()
		logger.Error("[ERROR]:Failed to read connect request:%v", err)
		return
	}
	start := time.Now()
	logger.Debug("[%d]Start handle stream:%v with comprresor:%s", stream.StreamID(), creq, ctx.auth.CompressMethod)
	if !defaultProxyLimitConfig.Allowed(creq.Addr) {
		logger.Error("'%s' is NOT allowed by proxy limit config.", creq.Addr)
		stream.Close()
		return
	}

	maxIdleTime := time.Duration(defaultMuxConfig.StreamIdleTimeout) * time.Second
	if maxIdleTime == 0 {
		maxIdleTime = 10 * time.Second
	}
	var c io.ReadWriteCloser
	dialTimeout := creq.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 10000
	}
	if len(creq.Hops) == 0 {
		var conn net.Conn
		conn, err = net.DialTimeout(creq.Network, creq.Addr, time.Duration(dialTimeout)*time.Millisecond)
		if nil != err {
			logger.Error("[ERROR]:Failed to connect %s:%v for reason:%v", creq.Network, creq.Addr, err)
		} else {
			if creq.ReadTimeout > 0 {
				//connection need to set read timeout to avoid hang forever
				readTimeout := time.Duration(creq.ReadTimeout) * time.Millisecond
				maxIdleTime = readTimeout
			}
			c = conn
		}
	} else {
		var nextURL *url.URL
		var nextStream mux.MuxStream
		next := creq.Hops[0]
		nextHops := creq.Hops[1:]
		nextURL, err = url.Parse(next)
		if nil == err {
			nextStream, _, err = GetMuxStreamByURL(nextURL, ctx.auth.User, &DefaultServerCipher)
			if nil == err {
				opt := mux.StreamOptions{
					DialTimeout: creq.DialTimeout,
					ReadTimeout: creq.ReadTimeout,
					Hops:        nextHops,
				}
				err = nextStream.Connect(creq.Network, creq.Addr, opt)
				if nil == err {
					c = nextStream
				} else {
					logger.Error("[ERROR]:Failed to connect next:%s for reason:%v", next, err)
				}
			}
		} else {
			logger.Error("Failed to parse proxy url:%s with reason:%v", next, err)
		}
	}

	if nil != err {
		stream.Close()
		return
	}
	streamReader, streamWriter := mux.GetCompressStreamReaderWriter(stream, ctx.auth.CompressMethod)
	defer c.Close()
	closeSig := make(chan bool, 1)

	go func() {
		//buf := make([]byte, 128*1024)
		//buf := upBytesPool.Get().([]byte)
		var buf []byte
		if _, ok := c.(io.ReaderFrom); !ok {
			buf = upBytesPool.Get().([]byte)
		}
		io.CopyBuffer(c, streamReader, buf)
		if len(buf) > 0 {
			upBytesPool.Put(buf)
		}
		logger.Debug("[%d]1Cost %v to handle stream:%v", stream.StreamID(), time.Now().Sub(start), creq)
		closeSig <- true
	}()

	var connReader io.Reader
	connReader = c
	rateLimitBucket := getRateLimitBucket(ctx.auth.User)
	if nil != rateLimitBucket {
		connReader = ratelimit.Reader(c, rateLimitBucket)
	}

	//buf := make([]byte, 128*1024)
	var buf []byte
	if _, ok := streamWriter.(io.ReaderFrom); !ok {
		buf = downBytesPool.Get().([]byte)
	}
	for {
		// if d, ok := c.(DeadLineAccetor); ok {
		// 	d.SetReadDeadline(time.Now().Add(maxIdleTime))
		// }
		n, err := io.CopyBuffer(streamWriter, connReader, buf)
		if isTimeoutErr(err) && time.Now().Sub(stream.LatestIOTime()) < maxIdleTime {
			continue
		}
		c.Close()
		stream.Close()
		logger.Debug("[%d]2Cost %v to handle stream:%v  with %d bytes err:%v", stream.StreamID(), time.Now().Sub(start), creq, n, err)
		break
	}
	if len(buf) > 0 {
		downBytesPool.Put(buf)
	}
	<-closeSig
	if close, ok := streamWriter.(io.Closer); ok {
		close.Close()
	}
	if close, ok := streamReader.(io.Closer); ok {
		close.Close()
	}
	logger.Debug("[%d]Cost %v to handle stream:%v ", stream.StreamID(), time.Now().Sub(start), creq)
}

var DefaultServerCipher CipherConfig

func serverAuthSession(session mux.MuxSession, raddr net.Addr, isFirst bool) (*mux.AuthRequest, error) {
	stream, err := session.AcceptStream()
	if nil != err {
		if err != pmux.ErrSessionShutdown {
			logger.Error("Failed to accept stream with error:%v", err)
		}
		return nil, err
	}
	recvAuth, err := mux.ReadAuthRequest(stream)
	if nil != err {
		logger.Error("[ERROR]:Failed to read auth request:%v", err)
		return nil, err
	}
	logger.Info("Recv auth:%v %v", recvAuth, isFirst)
	if !DefaultServerCipher.VerifyUser(recvAuth.User) {
		session.Close()
		return nil, mux.ErrAuthFailed
	}
	if !mux.IsValidCompressor(recvAuth.CompressMethod) {
		logger.Error("[ERROR]Invalid compressor:%s", recvAuth.CompressMethod)
		session.Close()
		return nil, mux.ErrAuthFailed
	}
	if len(recvAuth.P2PToken) > 0 {
		if !addP2PSession(recvAuth, session, raddr) {
			session.Close()
			return nil, mux.ErrAuthFailed
		}
		//ctx.isP2P = true
	}
	authRes := &mux.AuthResponse{
		Code: mux.AuthOK,
	}
	if len(recvAuth.P2PPriAddr) > 0 {
		peerPriAddr, peerPubAddr := getPeerAddr(recvAuth)
		logger.Info("Remote peer private addr:%v & public addr:%v for auth:%v", peerPriAddr, peerPubAddr, recvAuth)
		if len(peerPriAddr) > 0 {
			authRes.PeerPriAddr = peerPriAddr
			authRes.PeerPubAddr = peerPubAddr
		}
	}
	if len(recvAuth.P2PPubAddr) == 0 {
		if nil != raddr {
			authRes.PubAddr = raddr.String()
		}
	} else {
		authRes.PubAddr = recvAuth.P2PPubAddr
	}
	mux.WriteMessage(stream, authRes)
	stream.Close()
	if isFirst {
		if tmp, ok := session.(*mux.ProxyMuxSession); ok {
			tmp.Session.ResetCryptoContext(recvAuth.CipherMethod, recvAuth.CipherCounter)
		}
	}

	return recvAuth, nil
}

func ServProxyMuxSession(session mux.MuxSession, auth *mux.AuthRequest, raddr net.Addr) error {
	ctx := &sessionContext{}
	ctx.auth = auth
	ctx.activeIOTime = time.Now()
	ctx.session = session
	defer ctx.close()
	isFirst := true
	for {
		if nil == ctx.auth || ctx.isP2PExahnge {
			recvAuth, err := serverAuthSession(session, raddr, isFirst)
			if nil != err {
				return err
			}
			isFirst = false
			ctx.auth = recvAuth
			if len(recvAuth.P2PPriAddr) > 0 {
				ctx.isP2PExahnge = true
			}
			if len(recvAuth.P2PToken) > 0 {
				ctx.isP2P = true
			}
			continue
		}
		if 0 == atomic.LoadInt32(&ctx.streamCouter) {
			emptySessions.Store(ctx, true)
		}
		stream, err := session.AcceptStream()
		if nil != err {
			if err != pmux.ErrSessionShutdown {
				logger.Error("Failed to accept stream with error:%v", err)
			}
			return err
		}
		if ctx.isP2P {
			go handleP2PProxyStream(stream, ctx)
		} else {
			go handleProxyStream(stream, ctx)
		}
	}
}
