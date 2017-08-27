package ssh

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/getlantern/netx"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/proxy"
)

type sshStream struct {
	conn    net.Conn
	conf    *proxy.ProxyChannelConfig
	addr    string
	session *sshMuxSession
}

func (tc *sshStream) Auth(req *mux.AuthRequest) error {
	return nil
}

func (tc *sshStream) Connect(network string, addr string) error {
	switch network {
	case "tcp", "tcp6", "tcp4":
	default:
		return fmt.Errorf("No support for local proxy connections by network type:%s", network)
	}
	sshClient := tc.session.getSSHClient()
	if nil == sshClient {
		tc.session.Close()
		return fmt.Errorf("SSH connection closed")
	}
	conn, err := sshClient.Dial(network, addr)
	if nil != err {
		tc.session.Close()
		return err
	}
	tc.conn = conn
	tc.addr = addr
	return nil
}

func (tc *sshStream) StreamID() uint32 {
	return 0
}

func (tc *sshStream) Read(p []byte) (int, error) {
	if nil == tc.conn {
		return 0, io.EOF
	}
	return tc.conn.Read(p)
}
func (tc *sshStream) Write(p []byte) (int, error) {
	if nil == tc.conn {
		return 0, io.EOF
	}
	return tc.conn.Write(p)
}

func (tc *sshStream) Close() error {
	conn := tc.conn
	if nil != conn {
		conn.Close()
		tc.conn = nil
	}
	tc.session.closeStream(tc)
	return nil
}

type sshMuxSession struct {
	conf         *proxy.ProxyChannelConfig
	streams      map[*sshStream]bool
	streamsMutex sync.Mutex
	sshClient    *ssh.Client
}

func (tc *sshMuxSession) getSSHClient() *ssh.Client {
	return tc.sshClient
}

func (tc *sshMuxSession) closeStream(s *sshStream) {
	tc.streamsMutex.Lock()
	defer tc.streamsMutex.Unlock()
	delete(tc.streams, s)
}

func (tc *sshMuxSession) CloseStream(stream mux.MuxStream) error {
	//stream.Close()
	return nil
}

func (tc *sshMuxSession) OpenStream() (mux.MuxStream, error) {
	if nil == tc.sshClient {
		return nil, fmt.Errorf("SSH client closed")
	}
	tc.streamsMutex.Lock()
	defer tc.streamsMutex.Unlock()
	stream := &sshStream{
		conf:    tc.conf,
		session: tc,
	}
	return stream, nil
}
func (tc *sshMuxSession) AcceptStream() (mux.MuxStream, error) {
	return nil, nil
}

func (tc *sshMuxSession) NumStreams() int {
	tc.streamsMutex.Lock()
	defer tc.streamsMutex.Unlock()
	return len(tc.streams)
}

func (tc *sshMuxSession) Close() error {
	tc.streamsMutex.Lock()
	defer tc.streamsMutex.Unlock()
	for stream := range tc.streams {
		stream.Close()
	}
	tc.streams = make(map[*sshStream]bool)
	if nil != tc.sshClient {
		tc.sshClient.Close()
		tc.sshClient = nil
	}
	return nil
}

type SSHProxy struct {
}

func (p *SSHProxy) Features() proxy.ProxyFeatureSet {
	return proxy.ProxyFeatureSet{
		AutoExpire: true,
	}
}

func (p *SSHProxy) CreateMuxSession(server string, conf *proxy.ProxyChannelConfig) (mux.MuxSession, error) {
	u, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	addr := u.Host
	if nil != conf.ProxyURL() {
		addr = conf.ProxyURL().Host
	}
	connectHost, connectPort, _ := net.SplitHostPort(u.Host)
	if net.ParseIP(connectHost) == nil {
		iphost, err := proxy.DnsGetDoaminIP(connectHost)
		if nil != err {
			return nil, err
		}
		addr = net.JoinHostPort(iphost, connectPort)
	}
	dailTimeout := conf.DialTimeout
	if 0 == dailTimeout {
		dailTimeout = 5
	}
	//log.Printf("Session:%d connect %s:%s for %s %T %v %v %s", ev.GetId(), network, addr, host, ev, needHttpsConnect, conf.ProxyURL(), net.JoinHostPort(host, port))
	c, err := netx.DialTimeout("tcp", addr, time.Duration(dailTimeout)*time.Second)
	if nil != conf.ProxyURL() && nil == err {
		if strings.HasPrefix(conf.ProxyURL().Scheme, "socks") {
			err = helper.Socks5ProxyConnect(conf.ProxyURL(), c, net.JoinHostPort(connectHost, connectPort))
		} else {
			err = helper.HTTPProxyConnect(conf.ProxyURL(), c, "https://"+net.JoinHostPort(connectHost, connectPort))
		}
	}
	var sshConf *ssh.ClientConfig
	passwd, ok := u.User.Password()
	if ok {
		sshConf = &ssh.ClientConfig{
			User: u.User.Username(),
			Auth: []ssh.AuthMethod{
				ssh.Password(passwd),
			},
			HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return nil
			},
		}
	} else {
		if identify := u.Query().Get("key"); len(identify) > 0 {
			if content, err := ioutil.ReadFile(identify); nil != err {
				log.Printf("Invalid SSH identify path:%s for reason:%v", identify, err)
				return nil, err
			} else {
				signer, err := ssh.ParsePrivateKey(content)
				if nil != err {
					log.Printf("Invalid pem content for path:%s with reason:%v\n", identify, err)
					return nil, err
				}
				sshConf = &ssh.ClientConfig{
					User: u.User.Username(),
					Auth: []ssh.AuthMethod{
						ssh.PublicKeys(signer),
					},
					HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
						return nil
					},
				}
			}
		} else {
			return nil, fmt.Errorf("Can NOT connect ssh server:%s", server)
		}
	}

	conn, chans, reqs, err := ssh.NewClientConn(c, u.Host, sshConf)
	if nil != err {
		return nil, err
	}
	sClient := ssh.NewClient(conn, chans, reqs)
	session := &sshMuxSession{
		conf:      conf,
		streams:   make(map[*sshStream]bool),
		sshClient: sClient,
	}
	return session, nil
}

func init() {
	proxy.RegisterProxyType("ssh", &SSHProxy{})
}
