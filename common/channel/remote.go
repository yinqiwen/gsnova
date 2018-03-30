package channel

import (
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/pmux"

	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
)

type sessionContext struct {
	activeIOTime time.Time
	streamCouter int32
	session      mux.MuxSession
	closed       bool
}

func (ctx *sessionContext) close() {
	ctx.closed = true
	ctx.session.Close()
	emptySessions.Delete(ctx)
}

var emptySessions sync.Map

func init() {
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

func handleProxyStream(stream mux.MuxStream, auth *mux.AuthRequest, ctx *sessionContext) {
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
	logger.Debug("[%d]Start handle stream:%v with comprresor:%s", stream.StreamID(), creq, auth.CompressMethod)

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
			nextStream, _, err = GetMuxStreamByURL(nextURL, auth.User, &DefaultServerCipher)
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
	streamReader, streamWriter := mux.GetCompressStreamReaderWriter(stream, auth.CompressMethod)
	defer c.Close()
	closeSig := make(chan bool, 2)

	go func() {
		buf := make([]byte, 128*1024)
		io.CopyBuffer(c, streamReader, buf)
		closeSig <- true
	}()
	go func() {
		buf := make([]byte, 128*1024)
		for {
			if d, ok := c.(DeadLineAccetor); ok {
				d.SetReadDeadline(time.Now().Add(maxIdleTime))
			}
			_, err := io.CopyBuffer(streamWriter, c, buf)
			if isTimeoutErr(err) && time.Now().Sub(stream.LatestIOTime()) < maxIdleTime {
				continue
			}
			break
		}
		closeSig <- true
	}()

	<-closeSig

	if close, ok := streamWriter.(io.Closer); ok {
		close.Close()
	}
	if close, ok := streamReader.(io.Closer); ok {
		close.Close()
	}
}

var DefaultServerCipher CipherConfig

func ServProxyMuxSession(session mux.MuxSession) error {
	var authReq *mux.AuthRequest
	ctx := &sessionContext{}
	ctx.activeIOTime = time.Now()
	ctx.session = session
	defer ctx.close()

	for {
		stream, err := session.AcceptStream()
		if nil != err {
			//session.Close()
			if err != pmux.ErrSessionShutdown {
				logger.Error("Failed to accept stream with error:%v", err)
			}
			return err
		}
		if nil == authReq {
			auth, err := mux.ReadAuthRequest(stream)
			if nil != err {
				logger.Error("[ERROR]:Failed to read auth request:%v", err)
				continue
			}
			logger.Info("Recv auth:%v", auth)
			if !DefaultServerCipher.VerifyUser(auth.User) {
				session.Close()
				return mux.ErrAuthFailed
			}
			if !mux.IsValidCompressor(auth.CompressMethod) {
				logger.Error("[ERROR]Invalid compressor:%s", auth.CompressMethod)
				session.Close()
				return mux.ErrAuthFailed
			}
			authReq = auth
			authRes := &mux.AuthResponse{Code: mux.AuthOK}
			mux.WriteMessage(stream, authRes)
			stream.Close()
			if tmp, ok := session.(*mux.ProxyMuxSession); ok {
				tmp.Session.ResetCryptoContext(auth.CipherMethod, auth.CipherCounter)
			}
			continue
		}
		go handleProxyStream(stream, authReq, ctx)
	}
}
