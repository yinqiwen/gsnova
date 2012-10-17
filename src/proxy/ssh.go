package proxy

import (
	"bufio"
	"code.google.com/p/go.crypto/ssh"
	"common"
	"crypto"
	"crypto/dsa"
	"crypto/rsa"
	_ "crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"event"
	"fmt"
	"github.com/yinqiwen/godns"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"util"
)

var sshUseGlobalProxy bool
var sshResolveRemote bool

type SSHConnection struct {
	ssh_conn          *SSHRawConnection
	manager           *SSH
	proxy_conn        net.Conn
	proxy_conn_reader *bufio.Reader
	proxy_addr        string
}

func (conn *SSHConnection) initProxyConn(proxy_addr string, isHttps bool) error {
	if !strings.Contains(proxy_addr, ":") {
		if isHttps {
			proxy_addr = proxy_addr + ":443"
		} else {
			proxy_addr = proxy_addr + ":80"
		}
	}
	if nil != conn.proxy_conn && conn.proxy_addr == proxy_addr {
		return nil
	}
	conn.Close()
	conn.proxy_addr = proxy_addr
	ssh_conn, err := conn.ssh_conn.GetClientConn(false)
	if nil != err {
		if ssh_conn, err = conn.ssh_conn.GetClientConn(true); nil != err {
			return err
		}
	}
	var raddr *net.TCPAddr
	if !sshResolveRemote {
		raddr, err = net.ResolveTCPAddr("tcp", proxy_addr)
	} else {
		var host, port string
		var ips []net.IP
		host, port, err = net.SplitHostPort(proxy_addr)
		port_int, _ := strconv.Atoi(port)
		if nil == err {
			ips, err = conn.ssh_conn.RemoteResolve(host)
			if nil == err && len(ips) > 0 {
				raddr = &net.TCPAddr{
					IP:   ips[0],
					Port: port_int,
				}
			}
			if len(ips) == 0 && nil == err {
				err = errors.New(fmt.Sprintf("No DNS records found for %s", host))
			}
		}
	}
	if nil != err {
		return err
	}
	conn.proxy_conn, err = ssh_conn.DialTCP("tcp", nil, raddr)
	if nil == err && !isHttps {
		conn.proxy_conn_reader = bufio.NewReader(conn.proxy_conn)
	}
	return err
}

func (conn *SSHConnection) Request(sess *SessionConnection, ev event.Event) (err error, res event.Event) {
	f := func(local, remote net.Conn, ch chan int) {
		io.Copy(remote, local)
		local.Close()
		remote.Close()
		ch <- 1
	}
	switch ev.GetType() {
	case event.HTTP_REQUEST_EVENT_TYPE:
		req := ev.(*event.HTTPRequestEvent)
		if err := conn.initProxyConn(req.RawReq.Host, sess.Type == HTTPS_TUNNEL); nil != err {
			return err, nil
		}
		if sess.Type == HTTPS_TUNNEL {
			log.Printf("Session[%d]Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
			sess.LocalRawConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
			ch := make(chan int)
			go f(sess.LocalRawConn, conn.proxy_conn, ch)
			go f(conn.proxy_conn, sess.LocalRawConn, ch)
			<-ch
			<-ch
			conn.Close()
			sess.State = STATE_SESSION_CLOSE
		} else {
			log.Printf("Session[%d]Request %s\n", req.GetHash(), util.GetURLString(req.RawReq, true))
			err := req.RawReq.Write(conn.proxy_conn)
			if nil != err {
				return err, nil
			}
			resp, err := http.ReadResponse(conn.proxy_conn_reader, req.RawReq)
			if err != nil {
				return err, nil
			}
			err = resp.Write(sess.LocalRawConn)
			if nil == err {
				err = resp.Body.Close()
			}
			if nil != err || !util.IsResponseKeepAlive(resp) || !util.IsRequestKeepAlive(req.RawReq) {
				sess.LocalRawConn.Close()
				conn.Close()
				sess.State = STATE_SESSION_CLOSE
			} else {
				sess.State = STATE_RECV_HTTP
			}
		}
	default:
	}
	return nil, nil
}
func (conn *SSHConnection) GetConnectionManager() RemoteConnectionManager {
	return conn.manager
}
func (conn *SSHConnection) Close() error {
	if nil != conn.proxy_conn {
		conn.proxy_conn.Close()
		conn.proxy_conn = nil
	}
	return nil
}

type SSH struct {
	selector util.ListSelector
}

func (ssh *SSH) RecycleRemoteConnection(conn RemoteConnection) {
}

func (ssh *SSH) GetRemoteConnection(ev event.Event, attrs []string) (RemoteConnection, error) {
	conn := &SSHConnection{manager: ssh,
		ssh_conn: ssh.selector.Select().(*SSHRawConnection)}
	return conn, nil
}

func (manager *SSH) GetName() string {
	return SSH_NAME
}

type SSHRawConnection struct {
	ClientConfig *ssh.ClientConfig
	Server       string
	clientConn   *ssh.ClientConn
}

func (conn *SSHRawConnection) RemoteResolve(name string) ([]net.IP, error) {
	if raw, err := conn.GetClientConn(false); nil == err {
		dial := func(net, addr string, timeout time.Duration) (net.Conn, error) {
			return raw.Dial(net, addr)
		}
		options := &godns.LookupOptions{
			DNSServers:  godns.GoogleDNSServers,
			Net:         "tcp",
			Cache:       true,
			DialTimeout: dial,
		}
		return godns.LookupIP(name, options)
	} else {
		return nil, err
	}
	return nil, nil
}

func (conn *SSHRawConnection) GetClientConn(reconnect bool) (*ssh.ClientConn, error) {
	if !reconnect && nil != conn.clientConn {
		return conn.clientConn, nil
	} else {
		if nil != conn.clientConn {
			conn.clientConn.Close()
		}
		conn.clientConn = nil
		dial := net.Dial
		if sshUseGlobalProxy {
			dial = func(network, addr string) (net.Conn, error) {
				return util.HttpTunnelDial(network, addr, common.LocalProxy)
			}
		}
		if c, err := dial("tcp", conn.Server); nil != err {
			return nil, err
		} else {
			conn.clientConn, err = ssh.Client(c, conn.ClientConfig)
			return conn.clientConn, err
		}
	}
	return nil, nil
}

type password string

func (p password) Password(user string) (string, error) {
	return string(p), nil
}

// keychain implements the ClientKeyring interface
type keychain struct {
	keys []interface{}
}

func (k *keychain) Key(i int) (interface{}, error) {
	if i < 0 || i >= len(k.keys) {
		return nil, nil
	}
	switch key := k.keys[i].(type) {
	case *rsa.PrivateKey:
		return &key.PublicKey, nil
	case *dsa.PrivateKey:
		return &key.PublicKey, nil
	}
	panic("unknown key type")
}

func (k *keychain) Sign(i int, rand io.Reader, data []byte) (sig []byte, err error) {
	hashFunc := crypto.SHA1
	h := hashFunc.New()
	h.Write(data)
	digest := h.Sum(nil)
	switch key := k.keys[i].(type) {
	case *rsa.PrivateKey:
		return rsa.SignPKCS1v15(rand, key, hashFunc, digest)
	case *dsa.PrivateKey:
		r, s, err := dsa.Sign(rand, key, digest)
		if nil == err {
			return append(r.Bytes(), s.Bytes()...), nil
		}
	}
	return nil, errors.New("ssh: unknown key type")
}

func InitSSH() error {
	if enable, exist := common.Cfg.GetIntProperty("SSH", "Enable"); exist {
		if enable == 0 {
			return nil
		}
	}

	log.Println("Init SSH.")
	if enable, exist := common.Cfg.GetIntProperty("SSH", "UseGlobalProxy"); exist {
		sshUseGlobalProxy = (enable != 0)
	}
	if enable, exist := common.Cfg.GetIntProperty("SSH", "RemoteResolve"); exist {
		sshResolveRemote = (enable != 0)
	}

	var manager SSH
	RegisteRemoteConnManager(&manager)

	index := 0
	for ; ; index = index + 1 {
		v, exist := common.Cfg.GetProperty("SSH", "Server["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		var ssh_conn SSHRawConnection
		if u, err := url.Parse(v); nil == err {
			ssh_conn.Server = u.Host
			if !strings.Contains(u.Host, ":") {
				ssh_conn.Server = net.JoinHostPort(u.Host, "22")
			}
			if u.User == nil {
				log.Printf("Invalid SSH server url:%s, no user found in url.\n", v)
				continue
			} else {
				if pass, exist := u.User.Password(); exist {
					ssh_conn.ClientConfig = &ssh.ClientConfig{
						User: u.User.Username(),
						Auth: []ssh.ClientAuth{
							ssh.ClientAuthPassword(password(pass)),
						},
					}
				} else {
					if identify := u.Query().Get("i"); len(identify) > 0 {
						if content, err := ioutil.ReadFile(identify); nil != err {
							log.Printf("Invalid SSH identify path:%s for reason:%v.\n", identify, err)
							continue
						} else {
							block, _ := pem.Decode([]byte(content))
							if nil == block {
								log.Printf("Invalid pem content for path:%s.\n", identify)
								continue
							}
							clientKeychain := new(keychain)
							if strings.Contains(block.Type, "RSA") {
								rsakey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
								if err != nil {
									log.Printf("Invalid RSA private key for %v.\n", err)
									continue
								}
								clientKeychain.keys = append(clientKeychain.keys, rsakey)
							} else {
								dsakey, err := util.DecodeDSAPrivateKEy(block.Bytes)
								if err != nil {
									log.Printf("Invalid DSA private key for %v.\n", err)
									continue
								}
								clientKeychain.keys = append(clientKeychain.keys, dsakey)
							}

							ssh_conn.ClientConfig = &ssh.ClientConfig{
								User: u.User.Username(),
								Auth: []ssh.ClientAuth{
									ssh.ClientAuthKeyring(clientKeychain),
								},
							}
						}

					} else {
						log.Printf("Invalid SSH server url:%s, no pass/identify found in url.\n", v)
						continue
					}

				}
			}
			if _, err := ssh_conn.GetClientConn(true); nil == err {
				manager.selector.Add(&ssh_conn)
				log.Printf("SSH server %s connected.\n", ssh_conn.Server)
			} else {
				log.Printf("Invalid SSH server url:%s to connect for reason:%v\n", v, err)
			}
		} else {
			log.Printf("Invalid SSH server url:%s for reason:%v\n", v, err)
		}
	}
	if index == 0 {
		return errors.New("No configed SSH server.")
	}

	return nil
}
