package channel

import (
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/logger"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/pmux"
)

var ErrNotSupportedOperation = errors.New("Not supported operation")

type ProxyLimitConfig struct {
	WhiteList []string
	BlackList []string
}

func (limit *ProxyLimitConfig) Allowed(host string) bool {
	if len(limit.WhiteList) == 0 && len(limit.BlackList) == 0 {
		return true
	}
	if len(limit.BlackList) > 0 {
		for _, rule := range limit.BlackList {
			if rule == "*" {
				return false
			} else {
				matched, _ := filepath.Match(rule, host)
				if matched {
					return false
				}
			}
		}
		return true
	}

	for _, rule := range limit.WhiteList {
		if rule == "*" {
			return true
		} else {
			matched, _ := filepath.Match(rule, host)
			if matched {
				return true
			}
		}
	}
	return false
}

type FeatureSet struct {
	AutoExpire bool
	Pingable   bool
}

type MuxConfig struct {
	MaxStreamWindow    string
	StreamMinRefresh   string
	StreamIdleTimeout  int
	SessionIdleTimeout int
	UpBufferSize       int
	DownBufferSize     int
}

func (m *MuxConfig) ToPMuxConf() *pmux.Config {
	cfg := pmux.DefaultConfig()
	cfg.EnableKeepAlive = false

	if len(m.MaxStreamWindow) > 0 {
		v, err := helper.ToBytes(m.MaxStreamWindow)
		if nil == err {
			cfg.MaxStreamWindowSize = uint32(v)
		}
	}
	if len(m.StreamMinRefresh) > 0 {
		v, err := helper.ToBytes(m.StreamMinRefresh)
		if nil == err {
			cfg.StreamMinRefresh = uint32(v)
		}
	}
	return cfg
}

type CipherConfig struct {
	User   string
	Method string
	Key    string

	allowedUser []string
}

func (conf *CipherConfig) Adjust() {
	if len(conf.Method) == 0 {
		conf.Method = "auto"
	}
	switch conf.Method {
	case "auto":
		if strings.Contains(runtime.GOARCH, "386") || strings.Contains(runtime.GOARCH, "amd64") {
			conf.Method = pmux.CipherAES256GCM
		} else if strings.Contains(runtime.GOARCH, "arm") {
			conf.Method = pmux.CipherChacha20Poly1305
		}
	case pmux.CipherChacha20Poly1305:
	case pmux.CipherSalsa20:
	case pmux.CipherAES256GCM:
	case pmux.CipherNone:
	default:
		logger.Error("Invalid encrypt method:%s, use 'chacha20poly1305' instead.", conf.Method)
		conf.Method = pmux.CipherChacha20Poly1305
	}
}

func (conf *CipherConfig) AllowUsers(users string) {
	if len(users) > 0 {
		conf.allowedUser = strings.Split(users, ",")
	}
}

func (conf *CipherConfig) VerifyUser(user string) bool {
	if len(conf.allowedUser) == 0 {
		return true
	}
	for _, u := range conf.allowedUser {
		if u == user || u == "*" {
			//log.Printf("Valid user:%s", user)
			return true
		}
	}
	logger.Error("[ERROR]Invalid user:%s", user)
	return false
}

type RateLimitConfig struct {
	Limit map[string]string
}

type HTTPBaseConfig struct {
	HTTPPushRateLimitPerSec int
	UserAgent               string
	ReadTimeout             int
}
type HTTPConfig struct {
	HTTPBaseConfig
}

func (hcfg *HTTPConfig) UnmarshalJSON(data []byte) error {
	hcfg.HTTPPushRateLimitPerSec = 3
	hcfg.ReadTimeout = 30000
	err := json.Unmarshal(data, &hcfg.HTTPBaseConfig)
	return err
}

type KCPBaseConfig struct {
	Mode         string
	Conn         int
	AutoExpire   int
	ScavengeTTL  int
	MTU          int
	SndWnd       int
	RcvWnd       int
	DataShard    int
	ParityShard  int
	DSCP         int
	AckNodelay   bool
	NoDelay      int
	Interval     int
	Resend       int
	NoCongestion int
	SockBuf      int
}

func (kcfg *KCPBaseConfig) InitDefaultConf() {
	kcfg.Mode = "fast"
	kcfg.Conn = 1
	kcfg.AutoExpire = 0
	kcfg.ScavengeTTL = 600
	kcfg.MTU = 1350
	kcfg.SndWnd = 128
	kcfg.RcvWnd = 512
	kcfg.DataShard = 10
	kcfg.ParityShard = 3
	kcfg.DSCP = 0
	kcfg.AckNodelay = true
	kcfg.NoDelay = 0
	kcfg.Interval = 50
	kcfg.Resend = 0
	kcfg.Interval = 50
	kcfg.NoCongestion = 0
	kcfg.SockBuf = 4194304
}

type KCPConfig struct {
	KCPBaseConfig
}

func (config *KCPConfig) adjustByMode() {
	switch config.Mode {
	case "normal":
		config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 40, 2, 1
	case "fast":
		config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 30, 2, 1
	case "fast2":
		config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 20, 2, 1
	case "fast3":
		config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 10, 2, 1
	}
}
func (kcfg *KCPConfig) UnmarshalJSON(data []byte) error {
	kcfg.KCPBaseConfig.InitDefaultConf()
	err := json.Unmarshal(data, &kcfg.KCPBaseConfig)
	if nil == err {
		kcfg.adjustByMode()
	}
	return err
}

type HopServers []string

func (i *HopServers) String() string {
	return "HopServers"
}

func (i *HopServers) Set(value string) error {
	*i = append(*i, value)
	return nil
}

type ProxyChannelConfig struct {
	Enable                 bool
	Name                   string
	ServerList             []string
	ConnsPerServer         int
	SNI                    []string
	SNIProxy               string
	Proxy                  string
	RemoteDialMSTimeout    int
	RemoteDNSReadMSTimeout int
	RemoteUDPReadMSTimeout int
	LocalDialMSTimeout     int
	ReconnectPeriod        int
	HeartBeatPeriod        int
	RCPRandomAdjustment    int
	Compressor             string
	KCP                    KCPConfig
	HTTP                   HTTPConfig
	Cipher                 CipherConfig
	Hops                   HopServers
	RemoteSNIProxy         map[string]string
	HibernateAfterSecs     int
	P2PToken               string
	P2S2PEnable            bool

	proxyURL    *url.URL
	lazyConnect bool
}

func (conf *ProxyChannelConfig) GetRemoteSNI(domain string) string {
	if nil != conf.RemoteSNIProxy {
		for k, v := range conf.RemoteSNIProxy {
			matched, err := filepath.Match(k, domain)
			if nil != err {
				logger.Error("Invalid pattern:%s with reason:%v", k, err)
				continue
			}
			if matched {
				return v
			}
		}
	}
	return ""
}

func (conf *ProxyChannelConfig) Adjust() {
	conf.Cipher.Adjust()
	if len(conf.KCP.Mode) == 0 {
		conf.KCP.InitDefaultConf()
	}
	conf.KCP.adjustByMode()
	if len(conf.Compressor) == 0 || !mux.IsValidCompressor(conf.Compressor) {
		conf.Compressor = mux.NoneCompressor
	}

	if conf.RCPRandomAdjustment > conf.ReconnectPeriod {
		conf.RCPRandomAdjustment = conf.ReconnectPeriod / 2
	}
	if conf.ConnsPerServer == 0 {
		conf.ConnsPerServer = 3
	}
	if 0 == conf.RemoteDNSReadMSTimeout {
		conf.RemoteDNSReadMSTimeout = 1000
	}
	if 0 == conf.RemoteUDPReadMSTimeout {
		conf.RemoteUDPReadMSTimeout = 15 * 1000
	}
	if 0 == conf.LocalDialMSTimeout {
		conf.LocalDialMSTimeout = 5000
	}
	if 0 == conf.HTTP.ReadTimeout {
		conf.HTTP.ReadTimeout = 30000
	}
	if 0 == conf.RemoteDialMSTimeout {
		conf.RemoteDialMSTimeout = 5000
	}
	if 0 == conf.HibernateAfterSecs {
		conf.HibernateAfterSecs = 1800
	}
}

func (c *ProxyChannelConfig) ProxyURL() *url.URL {
	if nil != c.proxyURL {
		return c.proxyURL
	}
	if len(c.Proxy) > 0 {
		var err error
		c.proxyURL, err = url.Parse(c.Proxy)
		if nil != err {
			logger.Error("Failed to parse proxy URL ", c.Proxy)
		}
	}
	return c.proxyURL
}

//var DefaultCipherKey string
var defaultMuxConfig MuxConfig
var defaultProxyLimitConfig ProxyLimitConfig

func SetDefaultMuxConfig(cfg MuxConfig) {
	defaultMuxConfig = cfg
}
func SetDefaultProxyLimitConfig(cfg ProxyLimitConfig) {
	defaultProxyLimitConfig = cfg
}

func InitialPMuxConfig(cipher *CipherConfig) *pmux.Config {
	//cfg := pmux.DefaultConfig()
	cfg := defaultMuxConfig.ToPMuxConf()
	cfg.CipherKey = []byte(cipher.Key)
	cfg.CipherMethod = mux.DefaultMuxCipherMethod
	cfg.CipherInitialCounter = mux.DefaultMuxInitialCipherCounter
	//cfg.EnableKeepAlive = false
	//cfg.PingTimeout = 5 * time.Second
	return cfg
}
