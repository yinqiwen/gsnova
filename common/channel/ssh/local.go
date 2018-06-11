package ssh

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/yinqiwen/gsnova/common/channel"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
)

type sshStream struct {
	net.Conn
	conf         *channel.ProxyChannelConfig
	addr         string
	session      *sshMuxSession
	latestIOTime time.Time
}

func (tc *sshStream) SetReadDeadline(t time.Time) error {
	if nil == tc.Conn {
		return io.EOF
	}
	return tc.Conn.SetReadDeadline(t)
}
func (tc *sshStream) SetWriteDeadline(t time.Time) error {
	if nil == tc.Conn {
		return io.EOF
	}
	return tc.Conn.SetWriteDeadline(t)
}

func (s *sshStream) LatestIOTime() time.Time {
	return s.latestIOTime
}

func (tc *sshStream) Auth(req *mux.AuthRequest) *mux.AuthResponse {
	return &mux.AuthResponse{Code: mux.AuthOK}
}

func (tc *sshStream) Connect(network string, addr string, opt mux.StreamOptions) error {
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
	tc.Conn = conn
	tc.addr = addr
	return nil
}

func (tc *sshStream) StreamID() uint32 {
	return 0
}

func (tc *sshStream) Read(p []byte) (int, error) {
	if nil == tc.Conn {
		return 0, io.EOF
	}
	tc.latestIOTime = time.Now()
	return tc.Conn.Read(p)
}
func (tc *sshStream) Write(p []byte) (int, error) {
	if nil == tc.Conn {
		return 0, io.EOF
	}
	tc.latestIOTime = time.Now()
	return tc.Conn.Write(p)
}

func (tc *sshStream) Close() error {
	conn := tc.Conn
	if nil != conn {
		conn.Close()
		tc.Conn = nil
	}
	tc.session.closeStream(tc)
	return nil
}

type sshMuxSession struct {
	conf         *channel.ProxyChannelConfig
	streams      map[*sshStream]bool
	streamsMutex sync.Mutex
	sshClient    *ssh.Client
}

func (s *sshMuxSession) RemoteAddr() net.Addr {
	return nil
}
func (s *sshMuxSession) LocalAddr() net.Addr {
	return nil
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

func (tc *sshMuxSession) Ping() (time.Duration, error) {
	if nil != tc.sshClient {
		start := time.Now()
		_, _, err := tc.sshClient.SendRequest("ping", true, nil)
		return time.Now().Sub(start), err
	}
	return 0, nil
}

type SSHProxy struct {
}

func (p *SSHProxy) Features() channel.FeatureSet {
	return channel.FeatureSet{
		AutoExpire: true,
		Pingable:   true,
	}
}

func (p *SSHProxy) CreateMuxSession(server string, conf *channel.ProxyChannelConfig) (mux.MuxSession, error) {
	u, err := url.Parse(server)
	if nil != err {
		return nil, err
	}
	c, err := channel.DialServerByConf(server, conf)
	if err != nil {
		return nil, err
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
				logger.Error("Invalid SSH identify path:%s for reason:%v", identify, err)
				return nil, err
			} else {
				signer, err := ssh.ParsePrivateKey(content)
				if nil != err {
					logger.Error("Invalid pem content for path:%s with reason:%v\n", identify, err)
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
	channel.RegisterLocalChannelType("ssh", &SSHProxy{})
}
