package proxy

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yinqiwen/gsnova/common/gfwlist"
	"github.com/yinqiwen/gsnova/common/helper"
	"github.com/yinqiwen/gsnova/common/mux"
	"github.com/yinqiwen/gsnova/local/hosts"
	"github.com/yinqiwen/pmux"
)

var GConf LocalConfig
var mygfwlist *gfwlist.GFWList
var cnIPRange *IPRangeHolder

const (
	BlockedByGFWRule = "BlockedByGFW"
	InHostsRule      = "InHosts"
	IsCNIPRule       = "IsCNIP"
)

func matchHostnames(pattern, host string) bool {
	host = strings.TrimSuffix(host, ".")
	pattern = strings.TrimSuffix(pattern, ".")

	if len(pattern) == 0 || len(host) == 0 {
		return false
	}

	patternParts := strings.Split(pattern, ".")
	hostParts := strings.Split(host, ".")

	if len(patternParts) != len(hostParts) {
		return false
	}

	for i, patternPart := range patternParts {
		if i == 0 && patternPart == "*" {
			continue
		}
		if patternPart != hostParts[i] {
			return false
		}
	}
	return true
}

type HTTPBaseConfig struct {
	HTTPPushRateLimitPerSec int
}
type HTTPConfig struct {
	HTTPBaseConfig
}

func (hcfg *HTTPConfig) UnmarshalJSON(data []byte) error {
	hcfg.HTTPPushRateLimitPerSec = 3
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

type KCPConfig struct {
	KCPBaseConfig
}

func (kcfg *KCPConfig) initDefaultConf() {
	kcfg.Mode = "fast"
	kcfg.Conn = 1
	kcfg.AutoExpire = 0
	kcfg.ScavengeTTL = 600
	kcfg.MTU = 1350
	kcfg.SndWnd = 128
	kcfg.RcvWnd = 512
	kcfg.DataShard = 10
	kcfg.ParityShard = 3
	kcfg.DSCP = 30
	kcfg.AckNodelay = true
	kcfg.NoDelay = 0
	kcfg.Interval = 50
	kcfg.Resend = 0
	kcfg.Interval = 50
	kcfg.NoCongestion = 0
	kcfg.SockBuf = 4194304
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
	kcfg.initDefaultConf()
	err := json.Unmarshal(data, &kcfg.KCPBaseConfig)
	if nil == err {
		kcfg.adjustByMode()
	}
	return err
}

type ProxyChannelConfig struct {
	Enable              bool
	Name                string
	ServerList          []string
	ConnsPerServer      int
	SNI                 []string
	SNIProxy            string
	Proxy               string
	DialTimeout         int
	ReadTimeout         int
	ReconnectPeriod     int
	HeartBeatPeriod     int
	RCPRandomAdjustment int
	Compressor          string
	KCP                 KCPConfig
	HTTP                HTTPConfig

	proxyURL *url.URL
}

func (c *ProxyChannelConfig) ProxyURL() *url.URL {
	if nil != c.proxyURL {
		return c.proxyURL
	}
	if len(c.Proxy) > 0 {
		var err error
		c.proxyURL, err = url.Parse(c.Proxy)
		if nil != err {
			log.Printf("Failed to parse proxy URL ", c.Proxy)
		}
	}
	return c.proxyURL
}

type PACConfig struct {
	Method   []string
	Host     []string
	URL      []string
	Rule     []string
	Protocol []string
	Remote   string
}

func (pac *PACConfig) ruleInHosts(req *http.Request) bool {
	return hosts.InHosts(req.Host)
}

func (pac *PACConfig) matchProtocol(protocol string) bool {
	if len(pac.Protocol) == 0 {
		return true
	}
	for _, p := range pac.Protocol {
		if p == "*" || strings.EqualFold(p, protocol) {
			return true
		}
	}
	return false
}

func (pac *PACConfig) matchRules(ip string, req *http.Request) bool {
	if len(pac.Rule) == 0 {
		return true
	}

	ok := true

	for _, rule := range pac.Rule {
		not := false
		if strings.HasPrefix(rule, "!") {
			not = true
			rule = rule[1:]
		}
		if strings.EqualFold(rule, InHostsRule) {
			if nil == req {
				ok = false
			} else {
				ok = pac.ruleInHosts(req)
			}
		} else if strings.EqualFold(rule, BlockedByGFWRule) {
			if nil != mygfwlist && nil != req {
				ok = mygfwlist.IsBlockedByGFW(req)
				if !ok {
					log.Printf("#### %s is NOT BlockedByGFW", req.Host)
				}
			} else {
				ok = true
				log.Printf("NIL GFWList object or request")
			}
		} else if strings.EqualFold(rule, IsCNIPRule) {
			if len(ip) == 0 || nil == cnIPRange {
				log.Printf("NIL CNIP content  or IP/Domain")
				ok = false
			} else {
				var err error
				if net.ParseIP(ip) == nil {
					ip, err = DnsGetDoaminIP(ip)
				}
				if nil == err {
					_, err = cnIPRange.FindCountry(ip)
				} else {
					log.Printf("######err:%v", err)
				}
				ok = (nil == err)
				log.Printf("ip:%s is CNIP:%v", ip, ok)
			}

		} else {
			log.Printf("###Invalid rule:%s", rule)
		}
		if not {
			ok = ok != true
		}
		if !ok {
			break
		}
	}
	return ok
}

func MatchPatterns(str string, rules []string) bool {
	if len(rules) == 0 {
		return true
	}
	str = strings.ToLower(str)
	for _, pattern := range rules {
		matched, err := filepath.Match(pattern, str)
		if nil != err {
			log.Printf("Invalid pattern:%s with reason:%v", pattern, err)
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

func (pac *PACConfig) Match(protocol string, ip string, req *http.Request) bool {
	ret := pac.matchProtocol(protocol)
	if !ret {
		return false
	}
	ret = pac.matchRules(ip, req)
	if !ret {
		return false
	}
	if nil == req {
		if len(pac.Host) > 0 || len(pac.Method) > 0 || len(pac.URL) > 0 {
			return false
		}
		return true
	}
	host := req.Host
	if len(pac.Host) > 0 && strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}
	return MatchPatterns(host, pac.Host) && MatchPatterns(req.Method, pac.Method) && MatchPatterns(req.URL.String(), pac.URL)
}

type ProxyConfig struct {
	Local string
	PAC   []PACConfig
}

func (cfg *ProxyConfig) getProxyChannelByHost(proto string, host string) string {
	creq, _ := http.NewRequest("Connect", "https://"+host, nil)
	return cfg.findProxyChannelByRequest(proto, host, creq)
}

func (cfg *ProxyConfig) findProxyChannelByRequest(proto string, ip string, req *http.Request) string {
	var channel string
	if len(ip) > 0 && helper.IsPrivateIP(ip) {
		//channel = "direct"
		return directProxyChannelName
	}
	for _, pac := range cfg.PAC {
		if pac.Match(proto, ip, req) {
			channel = pac.Remote
			break
		}
	}
	if len(channel) == 0 {
		log.Printf("No proxy channel found.")
	}
	return channel
}

type CipherConfig struct {
	Method string
	Key    string
}

type LocalDNSConfig struct {
	Listen     string
	TrustedDNS []string
	FastDNS    []string
	TCPConnect bool
	CacheSize  int
}

type AdminConfig struct {
	Listen        string
	BroadcastAddr string
	ConfigDir     string
}

type UDPGWConfig struct {
	Addr           string
	LocalDNSRecord map[string]string
}

func (gw *UDPGWConfig) matchDNS(domain string) string {
	if nil == gw.LocalDNSRecord {
		return ""
	}
	for k, v := range gw.LocalDNSRecord {
		matched, err := filepath.Match(k, domain)
		if nil != err {
			log.Printf("Invalid pattern:%s with reason:%v", k, err)
			continue
		}
		if matched {
			return v
		}
	}
	return ""
}

type GFWListConfig struct {
	URL      string
	UserRule []string
	Proxy    string
}

type LocalConfig struct {
	Log       []string
	Cipher    CipherConfig
	UserAgent string
	User      string
	LocalDNS  LocalDNSConfig
	UDPGW     UDPGWConfig
	Admin     AdminConfig
	GFWList   GFWListConfig
	Proxy     []ProxyConfig
	Channel   []ProxyChannelConfig
}

func (cfg *LocalConfig) init() error {
	gfwlistEnable := false
	cnIPEnable := false
	for i, _ := range cfg.Proxy {
		for j, _ := range cfg.Proxy[i].PAC {
			rules := cfg.Proxy[i].PAC[j].Rule
			for _, r := range rules {
				if strings.Contains(r, BlockedByGFWRule) || strings.Contains(r, IsCNIPRule) {
					gfwlistEnable = true
				}
				if strings.Contains(r, IsCNIPRule) {
					cnIPEnable = true
				}
			}
		}
	}

	if gfwlistEnable {
		go func() {
			hc, _ := NewHTTPClient(&ProxyChannelConfig{Proxy: cfg.GFWList.Proxy})
			hc.Timeout = 15 * time.Second
			dst := proxyHome + "/gfwlist.txt"
			tmp, err := gfwlist.NewGFWList(cfg.GFWList.URL, hc, cfg.GFWList.UserRule, dst, true)
			if nil == err {
				mygfwlist = tmp
			} else {
				log.Printf("[ERROR]Failed to create gfwlist  for reason:%v", err)
			}
		}()
	}
	if cnIPEnable {
		go func() {
			iprangeFile := proxyHome + "/" + cnIPFile
			ipHolder, err := parseApnicIPFile(iprangeFile)
			nextFetchTime := 1 * time.Second
			if nil == err {
				cnIPRange = ipHolder
				nextFetchTime = 1 * time.Minute
			}
			var hc *http.Client
			for {
				select {
				case <-time.After(nextFetchTime):
					if nil == hc {
						hc, err = NewHTTPClient(&ProxyChannelConfig{})
						hc.Timeout = 15 * time.Second
					}
					if nil != hc {
						ipHolder, err = getCNIPRangeHolder(hc)
						if nil != err {
							log.Printf("[ERROR]Failed to fetch CNIP file:%v", err)
							nextFetchTime = 1 * time.Second
						} else {
							nextFetchTime = 24 * time.Hour
							cnIPRange = ipHolder
						}
					}
				}
			}
		}()
	}

	switch GConf.Cipher.Method {
	case "auto":
		if strings.Contains(runtime.GOARCH, "386") || strings.Contains(runtime.GOARCH, "amd64") {
			GConf.Cipher.Method = pmux.CipherAES256GCM
		} else if strings.Contains(runtime.GOARCH, "arm") {
			GConf.Cipher.Method = pmux.CipherChacha20Poly1305
		}
	case pmux.CipherChacha20Poly1305:
	case pmux.CipherSalsa20:
	case pmux.CipherAES256GCM:
	case pmux.CipherNone:
	default:
		log.Printf("Invalid encrypt method:%s, use 'chacha20poly1305' instead.", GConf.Cipher.Method)
		GConf.Cipher.Method = pmux.CipherChacha20Poly1305
	}
	haveDirect := false
	for i := range GConf.Channel {
		if GConf.Channel[i].Name == directProxyChannelName && GConf.Channel[i].Enable {
			haveDirect = true
			GConf.Channel[i].ServerList = []string{"direct://0.0.0.0:0"}
			GConf.Channel[i].ConnsPerServer = 1
		} else {
			if len(GConf.Channel[i].Compressor) == 0 || !mux.IsValidCompressor(GConf.Channel[i].Compressor) {
				GConf.Channel[i].Compressor = mux.NoneCompressor
			}
		}

		if GConf.Channel[i].RCPRandomAdjustment > GConf.Channel[i].ReconnectPeriod {
			GConf.Channel[i].RCPRandomAdjustment = GConf.Channel[i].ReconnectPeriod / 2
		}
	}
	if !haveDirect {
		directProxyChannel := make([]ProxyChannelConfig, 1)
		directProxyChannel[0].Name = directProxyChannelName
		directProxyChannel[0].Enable = true
		directProxyChannel[0].ConnsPerServer = 1
		directProxyChannel[0].DialTimeout = 5
		directProxyChannel[0].ReadTimeout = 30
		directProxyChannel[0].ServerList = []string{"direct://0.0.0.0:0"}
		GConf.Channel = append(directProxyChannel, GConf.Channel...)
	}
	return nil
}
