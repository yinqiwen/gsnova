package channel

import (
	"net"
	"reflect"
	"sort"
	"time"

	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
)

type LocalChannel interface {
	//PrintStat(w io.Writer)
	CreateMuxSession(server string, conf *ProxyChannelConfig) (mux.MuxSession, error)
	Features() FeatureSet
}

var LocalChannelTypeTable map[string]reflect.Type = make(map[string]reflect.Type)

const DirectChannelName = "direct"

var DirectSchemes = []string{
	DirectChannelName,
	"socks",
	"socks4",
	"socks5",
	"http_proxy",
}

func IsDirectScheme(scheme string) bool {
	for _, s := range DirectSchemes {
		if s == scheme {
			return true
		}
	}
	return false
}

func RegisterLocalChannelType(str string, p LocalChannel) error {
	rt := reflect.TypeOf(p)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	LocalChannelTypeTable[str] = rt
	return nil
}

func AllowedSchema() []string {
	schemes := []string{}
	for scheme := range LocalChannelTypeTable {
		if !IsDirectScheme(scheme) {
			schemes = append(schemes, scheme)
		}
	}
	sort.Strings(schemes)
	return schemes
}

func clientAuthMuxSession(session mux.MuxSession, cipherMethod string, conf *ProxyChannelConfig, tunnelPriAddr, tunnelPubAddr string, isFirst bool, isP2P bool) (error, *mux.AuthRequest, *mux.AuthResponse) {
	authStream, err := session.OpenStream()
	if nil != err {
		return err, nil, nil
	}
	//counter := uint64(helper.RandBetween(0, math.MaxInt32))
	counter := uint64(1)
	authReq := &mux.AuthRequest{
		User:           conf.Cipher.User,
		CipherCounter:  counter,
		CipherMethod:   cipherMethod,
		CompressMethod: conf.Compressor,
		P2PToken:       conf.P2PToken,
		P2PPriAddr:     tunnelPriAddr,
		P2PPubAddr:     tunnelPubAddr,
	}
	if len(conf.P2PToken) > 0 {
		authReq.P2PConnID = p2pConnID
	}
	if isP2P {
		authReq.P2PToken = ""
		authReq.P2PConnID = ""
		authReq.P2PPriAddr = ""
		authReq.P2PPubAddr = ""
	}
	authStream.SetReadDeadline(time.Now().Add(3 * time.Second))
	authRes := authStream.Auth(authReq)
	err = authRes.Error()
	if nil != err {
		return err, nil, nil
	}
	//wait auth stream close
	var zero time.Time
	authStream.SetReadDeadline(zero)
	authStream.Read(make([]byte, 1))
	//authStream.Close()
	if isFirst {
		if psession, ok := session.(*mux.ProxyMuxSession); ok {
			err = psession.Session.ResetCryptoContext(cipherMethod, counter)
			if nil != err {
				logger.Error("[ERROR]Failed to reset cipher context with reason:%v, while cipher method:%s", err, cipherMethod)
				return err, nil, nil
			}
		}
	}
	return nil, authReq, authRes
}

func clientAuthConn(c net.Conn, cipherMethod string, conf *ProxyChannelConfig, p2pTunnel bool) (error, *mux.AuthRequest, *mux.AuthResponse, mux.MuxSession) {
	session, err := pmux.Client(c, InitialPMuxConfig(&conf.Cipher))
	if nil != err {
		logger.Error("Failed to init mux session:%v", err)
		c.Close()
		return err, nil, nil, nil
	}
	ps := &mux.ProxyMuxSession{Session: session}
	var tunnelPriAddr string
	if !p2pTunnel {
		tunnelPriAddr = c.LocalAddr().String()
	}
	err, req, res := clientAuthMuxSession(ps, cipherMethod, conf, tunnelPriAddr, "", true, p2pTunnel)
	if nil != err {
		logger.Error("Failed to auth mux session:%v", err)
		c.Close()
		return err, nil, nil, nil
	}
	return nil, req, res, ps
}
