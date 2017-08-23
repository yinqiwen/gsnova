package kcp

import (
	"log"
	"net"
	"net/url"

	kcp "github.com/xtaci/kcp-go"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/proxy"
	"github.com/yinqiwen/pmux"
)

type KCPProxy struct {
	//proxy.BaseProxy
}

func (tc *KCPProxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	rurl, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	hostport := rurl.Host
	tcpHost, tcpPort, _ := net.SplitHostPort(hostport)
	if net.ParseIP(tcpHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(tcpHost)
		if nil != err {
			return nil, err
		}
		hostport = net.JoinHostPort(iphost, tcpPort)
	}
	block, _ := kcp.NewNoneBlockCrypt(nil)
	kcpconn, err := kcp.DialWithOptions(hostport, block, tc.Conf.KCP.DataShard, tc.Conf.KCP.ParityShard)
	if err != nil {
		return nil, err
	}
	kcpconn.SetStreamMode(true)
	kcpconn.SetWriteDelay(true)
	kcpconn.SetNoDelay(conf.KCP.NoDelay, conf.KCP.Interval, conf.KCP.Resend, conf.KCP.NoCongestion)
	kcpconn.SetWindowSize(conf.KCP.SndWnd, conf.KCP.RcvWnd)
	kcpconn.SetMtu(conf.KCP.MTU)
	kcpconn.SetACKNoDelay(conf.KCP.AckNodelay)

	if err := kcpconn.SetDSCP(conf.KCP.DSCP); err != nil {
		log.Println("SetDSCP:", err)
	}
	if err := kcpconn.SetReadBuffer(conf.KCP.SockBuf); err != nil {
		log.Println("SetReadBuffer:", err)
	}
	if err := kcpconn.SetWriteBuffer(conf.KCP.SockBuf); err != nil {
		log.Println("SetWriteBuffer:", err)
	}
	session, err := pmux.Client(kcpconn, proxy.InitialPMuxConfig())
	if nil != err {
		return nil, err
	}
	log.Printf("Connect %s success.", server)
	return &mux.ProxyMuxSession{Session: session}, nil
}

func init() {
	proxy.RegisterProxyType("kcp", &KCPProxy{})
}
