package proxy

import (
	"bufio"
	"common"
	"encoding/json"
	"github.com/yinqiwen/godns"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"util"
)

const (
	DNS_CACHE_FILE     = "DNSCache.json"
	USER_HOSTS_FILE    = "user_hosts.conf"
	CLOUD_HOSTS_FILE   = "cloud_hosts.conf"
	HOSTS_DISABLE      = 0
	HOSTS_ENABLE_HTTPS = 1
	HOSTS_ENABLE_ALL   = 2
)

type DNSResult struct {
	IP   []string
	Date time.Time
}

var repoUrls []string
var hostMapping = make(map[string]string)
var reachableDNSResult = make(map[string]DNSResult)
var reachableDNSResultMutex sync.Mutex
var dnsResultChanged = false
var blockVerifyResult = make(map[string]bool)
var blockVerifyResultMutex sync.Mutex
var preferDNS = true
var dnsCacheExpire = uint32(604800)
var persistDNSCache = true

var hostsEnable int
var trustedDNS = []string{}
var useHttpDNS = []*regexp.Regexp{}

//var forceHttpsHosts = []*regexp.Regexp{}
var exceptHosts = []*regexp.Regexp{}
var httpDNS string
var blockVerifyTimeout = 5

var hostRangeFetchLimitSize = uint32(256000)
var hostInjectRangePatterns = []*regexp.Regexp{}
var hostRangeConcurrentFether = uint32(5)

func loadIPRangeFile(ipRepo string) {
	if len(ipRepo) == 0 {
		return
	}
	time.Sleep(5*time.Second)
	hf := common.Home + "hosts/" + "iprange.zip"
	_, err := os.Stat(hf)
	if nil != err {
		body, _, err := util.FetchLateastContent(ipRepo, common.ProxyPort, true)
		if err != nil {
			log.Printf("Failed to fetch ip range file from %s for reason:%v\n", ipRepo, err)
			return
		} else {
			err = ioutil.WriteFile(hf, body, 0755)
			if nil != err {
				log.Printf("Failed to manipulate ip range file for reason:%v\n", err)
				return
			}
		}
		log.Printf("Fetch ip range file success.\n")
	}
	init_iprange_func(hf)
}

func loadDiskHostFile() {
	files, err := ioutil.ReadDir(common.Home + "hosts/")
	if nil == err {
		for _, file := range files {
			switch file.Name() {
			case USER_HOSTS_FILE, CLOUD_HOSTS_FILE:
				continue
			}
			content, err := ioutil.ReadFile(common.Home + "hosts/" + file.Name())
			if nil == err {
				if strings.EqualFold(DNS_CACHE_FILE, file.Name()) {
					if persistDNSCache {
						json.Unmarshal(content, &reachableDNSResult)
						for k, v := range reachableDNSResult {
							_, port, _ := net.SplitHostPort(k)
							for _, ip := range v.IP {
								//blockVerifyResult[net.JoinHostPort(ip, port)] = true
								setBlockVerifyCacheResult(net.JoinHostPort(ip, port), true)
							}
						}
					}
					continue
				}
				reader := bufio.NewReader(strings.NewReader(string(content)))
				for {
					line, _, err := reader.ReadLine()
					if nil != err {
						break
					}
					str := string(line)
					str = strings.TrimSpace(str)

					if strings.HasPrefix(str, "#") || len(str) == 0 {
						continue
					}
					ss := strings.Split(str, " ")
					if len(ss) == 1 {
						ss = strings.Split(str, "\t")
					}
					if len(ss) == 2 {
						k := strings.TrimSpace(ss[1])
						v := strings.TrimSpace(ss[0])
						if !isExceptHost(k) {
							hostMapping[k] = v
						}
					}
				}
			}
		}
	}
}

func loadHostFile() {
	hostMapping = make(map[string]string)
	loadDiskHostFile()
	for index, urlstr := range repoUrls {
		resp, err := util.HttpGet(urlstr, "")
		if err != nil {
			if addr, exist := common.Cfg.GetProperty("LocalServer", "Listen"); exist {
				_, port, _ := net.SplitHostPort(addr)
				resp, err = util.HttpGet(urlstr, "http://"+net.JoinHostPort("127.0.0.1", port))
			}
		}
		if err != nil || resp.StatusCode != 200 {
			log.Printf("Failed to fetch host from %s\n", urlstr)
		} else {
			body, err := ioutil.ReadAll(resp.Body)
			if nil == err {
				hf := common.Home + "hosts/" + "hosts_" + strconv.Itoa(index) + ".txt"
				ioutil.WriteFile(hf, body, 0755)
			}
		}
	}
	loadDiskHostFile()
}

func isExceptHost(host string) bool {
	return hostPatternMatched(exceptHosts, host)
}

func persistDNSResult() {
	baseDuration := 1 * time.Minute
	tick := time.NewTicker(baseDuration)
	for {
		select {
		case <-tick.C:
			if !dnsResultChanged {
				break
			}
			if len(reachableDNSResult) > 0 {
				if content, err := marshalReachableDNSResult(); nil == err {
					filename := common.Home + "hosts/" + DNS_CACHE_FILE
					if f, err := os.OpenFile(filename, os.O_CREATE, 0666); nil == err {
						io.WriteString(f, string(content))
						f.Close()
					}
				}
			}

		}
	}
}

func lookupReachableAddress(hostport string) (string, bool) {
	host, port, err := net.SplitHostPort(hostport)
	if nil != err {
		return hostport, false
	}

	select_ip_from_dns := func() (string, bool) {
		if addrs, exist := trustedDNSQuery(host, port); exist {
			ips := make([]string, len(addrs))
			copy(ips, addrs)
			for _, ip := range ips {
				if !isTCPAddressBlocked(host, ip, port) {
					return net.JoinHostPort(ip, port), true
				}
			}
		}
		return "", false
	}
	if preferDNS {
		if addr, exist := select_ip_from_dns(); exist {
			return addr, true
		}
	}
	v, exist := getLocalHostMapping(host)
	if !exist {
		v, exist = hostMapping[host]
	}
	if exist && !isTCPAddressBlocked(host, v, port) {
		if preferDNS {
			go trustedDNSQuery(host, port)
		}
		return net.JoinHostPort(v, port), true
	}

	if !preferDNS {
		if addr, exist := select_ip_from_dns(); exist {
			return addr, true
		}
	}
	return hostport, false
}

func lookupReachableMappingHost(req *http.Request, hostport string) (string, bool) {
	switch hostsEnable {
	case HOSTS_DISABLE:
		return "", false
	case HOSTS_ENABLE_HTTPS:
		if !strings.EqualFold(req.Method, "Connect") {
			return "", false
		}
	}
	return lookupReachableAddress(hostport)
}

func marshalReachableDNSResult() ([]byte, error) {
	reachableDNSResultMutex.Lock()
	defer reachableDNSResultMutex.Unlock()
	dnsResultChanged = false
	return json.MarshalIndent(&reachableDNSResult, "", " ")
}

func getReachableDNSResult(hostport string) (v DNSResult, exist bool) {
	reachableDNSResultMutex.Lock()
	defer reachableDNSResultMutex.Unlock()
	v, exist = reachableDNSResult[hostport]
	return
}

func setReachableDNSResult(hostport string, v DNSResult) {
	reachableDNSResultMutex.Lock()
	reachableDNSResult[hostport] = v
	reachableDNSResultMutex.Unlock()
}

func expireReachableDNSResult(hostport string) {
	reachableDNSResultMutex.Lock()
	delete(reachableDNSResult, hostport)
	reachableDNSResultMutex.Unlock()
}

func expireBlockVerifyCache(hostport string) {
	blockVerifyResultMutex.Lock()
	delete(blockVerifyResult, hostport)
	blockVerifyResultMutex.Unlock()
	dnsResultChanged = true
}

func setBlockVerifyCacheResult(hostport string, result bool) {
	blockVerifyResultMutex.Lock()
	blockVerifyResult[hostport] = result
	blockVerifyResultMutex.Unlock()
	dnsResultChanged = true
}

func getBlockVerifyCacheResult(hostport string) (v, exist bool) {
	blockVerifyResultMutex.Lock()
	defer blockVerifyResultMutex.Unlock()
	v, exist = blockVerifyResult[hostport]
	return
}

func isTCPAddressBlocked(host, ip, port string) bool {
	addr := net.JoinHostPort(ip, port)
	if verify, exist := getBlockVerifyCacheResult(addr); exist {
		return verify
	}
	c, err := net.DialTimeout("tcp", addr, time.Duration(blockVerifyTimeout)*time.Second)
	if nil != err {
		delete(hostMapping, host)
		hostport := net.JoinHostPort(host, port)
		if addrs, exist := getReachableDNSResult(hostport); exist {
			for i, v := range addrs.IP {
				if v == ip {
					addrs.IP = append(addrs.IP[:i], addrs.IP[i+1:]...)
					dnsResultChanged = true
					setReachableDNSResult(hostport, addrs)
					break
				}
			}
		}
		//blockVerifyResult[addr] = true
		setBlockVerifyCacheResult(addr, true)
		return true
	}
	c.Close()
	//blockVerifyResult[addr] = false
	setBlockVerifyCacheResult(addr, false)
	return false
}

func hostNeedInjectRange(host string) bool {
	return hostPatternMatched(hostInjectRangePatterns, host)
}

func trustedDNSQuery(host string, port string) ([]string, bool) {
	hostport := net.JoinHostPort(host, port)
	if result, exist := getReachableDNSResult(hostport); exist {
		if time.Now().Sub(result.Date) >= time.Second*time.Duration(dnsCacheExpire) {
			//delete(reachableDNSResult, hostport)
			expireReachableDNSResult(hostport)
		} else {
			return result.IP, len(result.IP) > 0
		}
	}
	useTCPDns := (len(trustedDNS) > 0)
	if len(httpDNS) > 0 && hostPatternMatched(useHttpDNS, host) {
		useTCPDns = false
	}
	//Query DNS over TCP
	if useTCPDns {
		options := &godns.LookupOptions{
			DNSServers: trustedDNS,
			Net:        "tcp",
			Cache:      true}
		if ips, err := godns.LookupIP(host, options); nil == err {
			result := []string{}
			for _, ip := range ips {
				if nil != ip.To4() && !isTCPAddressBlocked(host, ip.String(), port) {
					result = append(result, ip.String())
				}
			}
			//dnsResultChanged = true
			//reachableDNSResult[hostport] = DNSResult{result, time.Now()}
			setReachableDNSResult(hostport, DNSResult{result, time.Now()})
			if len(result) > 0 {
				return result, true
			}
		}
	}
	//Query http DNS
	if len(httpDNS) > 0 {
		url := strings.Replace(httpDNS, "${DOMAIN}", host, 1)
		resp, err := http.DefaultClient.Get(url)
		if nil == err && resp.StatusCode == 200 {
			if content, err := ioutil.ReadAll(resp.Body); nil == err {
				var ips []string
				e := json.Unmarshal([]byte(content), &ips)
				if nil == e {
					result := []string{}
					for _, ip := range ips {
						tmp := net.ParseIP(ip)
						if nil != tmp.To4() && !isTCPAddressBlocked(host, ip, port) {
							result = append(result, ip)
						}
					}

					//reachableDNSResult[hostport] = DNSResult{result, time.Now()}
					setReachableDNSResult(hostport, DNSResult{result, time.Now()})
					if len(result) > 0 {
						return result, true
					}
				}
			}
		}
	}
	setReachableDNSResult(hostport, DNSResult{[]string{}, time.Now()})
	//reachableDNSResult[hostport] = DNSResult{[]string{}, time.Now()}
	return nil, false
}

func InitHosts() error {
	loadLocalHostMappings()
	if cloud_hosts, exist := common.Cfg.GetProperty("Hosts", "CloudHosts"); exist {
		go fetchCloudHosts(cloud_hosts)
	}

	if enable, exist := common.Cfg.GetIntProperty("Hosts", "Enable"); exist {
		hostsEnable = int(enable)
		if enable == 0 {
			return nil
		}
	}
	log.Println("Init AutoHost.")
	os.Mkdir(common.Home+"hosts/", 0755)
	if dnsserver, exist := common.Cfg.GetProperty("Hosts", "TrustedDNS"); exist {
		trustedDNS = strings.Split(dnsserver, "|")
	}
	if expire, exist := common.Cfg.GetIntProperty("Hosts", "DNSCacheExpire"); exist {
		dnsCacheExpire = uint32(expire)
	}
	if timeout, exist := common.Cfg.GetIntProperty("Hosts", "BlockVerifyTimeout"); exist {
		blockVerifyTimeout = int(timeout)
	}
	if limit, exist := common.Cfg.GetIntProperty("Hosts", "RangeFetchLimitSize"); exist {
		hostRangeFetchLimitSize = uint32(limit)
	}
	if pattern, exist := common.Cfg.GetProperty("Hosts", "InjectRange"); exist {
		hostInjectRangePatterns = initHostMatchRegex(pattern)
	}
	if fetcher, exist := common.Cfg.GetIntProperty("Hosts", "RangeConcurrentFetcher"); exist {
		hostRangeConcurrentFether = uint32(fetcher)
	}

	//	if pattern, exist := common.Cfg.GetProperty("Hosts", "RedirectHttps"); exist {
	//		forceHttpsHosts = initHostMatchRegex(pattern)
	//	}

	if pattern, exist := common.Cfg.GetProperty("Hosts", "ExceptHosts"); exist {
		exceptHosts = initHostMatchRegex(pattern)
	}
	if pattern, exist := common.Cfg.GetProperty("Hosts", "UseHttpDNS"); exist {
		useHttpDNS = initHostMatchRegex(pattern)
	}
	if prefer, exist := common.Cfg.GetBoolProperty("Hosts", "PreferDNS"); exist {
		preferDNS = prefer
	}

	if url, exist := common.Cfg.GetProperty("Hosts", "HttpDNS"); exist {
		httpDNS = strings.TrimSpace(url)
	}
	if url, exist := common.Cfg.GetProperty("Hosts", "IPRangeRepo"); exist {
		go loadIPRangeFile(strings.TrimSpace(url))
	}
	if len(httpDNS) > 0 || len(trustedDNS) > 0 {
		if enable, exist := common.Cfg.GetBoolProperty("Hosts", "PersistDNSCache"); exist {
			persistDNSCache = enable
			if persistDNSCache {
				go persistDNSResult()
			}
		}

	}
	repoUrls = make([]string, 0)
	index := 0
	for {
		v, exist := common.Cfg.GetProperty("Hosts", "HostsRepo["+strconv.Itoa(index)+"]")
		if !exist || len(v) == 0 {
			break
		}
		repoUrls = append(repoUrls, v)
		index++
	}
	go loadHostFile()
	return nil
}
