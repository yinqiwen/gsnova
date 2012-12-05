package main

import (
	"common"
	"event"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"proxy"
	"remote"
	"runtime"
	"sync/atomic"
	"time"
	"util"
)

const (
	MAX_READ_CHUNK_SIZE = 8192
)

var seed uint32 = 0

func handleConn(conn *net.TCPConn, proxyServerType int) {
	sessionID := atomic.AddUint32(&seed, 1)
	proxy.HandleConn(sessionID, conn, proxyServerType)
}

func handleServer(lp *net.TCPListener, proxyServerType int) {
	for {
		conn, err := lp.AcceptTCP()
		if nil != err {
			continue
		}
		go handleConn(conn, proxyServerType)
	}
}

func startLocalProxyServer(addr string, proxyServerType int) bool {
	tcpaddr, err := net.ResolveTCPAddr("tcp", addr)
	if nil != err {
		return false
	}
	var lp *net.TCPListener
	lp, err = net.ListenTCP("tcp", tcpaddr)
	if nil != err {
		log.Fatalf("Can NOT listen on address:%s\n", addr)
		return false
	}
	log.Printf("Listen on address %s\n", addr)
	handleServer(lp, proxyServerType)
	return true
}

func main() {
	path, err := filepath.Abs(os.Args[0])
	if nil != err {
		fmt.Println(err)
		return
	}
	as_server := flag.Bool("server", false, "Run as remote proxy server")
	event.Init()
	flag.Parse()
	if *as_server {
		remote.LaunchC4HttpServer()
		return
	}
	common.Home, _ = filepath.Split(path)
	common.InitLogger()
	common.InitConfig()
	proxy.InitHosts()
	proxy.InitSpac()
	proxy.InitGoogle()
	runtime.GOMAXPROCS(runtime.NumCPU())

	var gae proxy.GAE
	var c4 proxy.C4

	err = c4.Init()
	if nil != err {
		log.Printf("[WARN]Failed to init C4:%s\n", err.Error())
	}

	err = proxy.InitSSH()
	if nil != err {
		log.Printf("[WARN]Failed to init SSH:%s\n", err.Error())
	}

	err = gae.Init()
	if nil != err {
		log.Printf("[WARN]Failed to init GAE:%s\n", err.Error())
	}
	proxy.InitSelfWebServer()
	proxy.PostInitSpac()

    log.Printf("=============Start %s %s==============\n", common.Product, common.Version)
	if proxy.C4Enable {
		if addr, exist := common.Cfg.GetProperty("C4", "Listen"); exist {
			go startLocalProxyServer(addr, proxy.C4_PROXY_SERVER)
		}
	}
	if proxy.SSHEnable {
		if addr, exist := common.Cfg.GetProperty("SSH", "Listen"); exist {
			go startLocalProxyServer(addr, proxy.SSH_PROXY_SERVER)
		}
	}
	if proxy.GAEEnable {
	    	//init fake cert if GAE inited success
		common.LoadRootCA()
		if addr, exist := common.Cfg.GetProperty("GAE", "Listen"); exist {
			go startLocalProxyServer(addr, proxy.GAE_PROXY_SERVER)
		}
	}
	addr, exist := common.Cfg.GetProperty("LocalServer", "Listen")
	if !exist {
		log.Fatalln("No config [LocalServer]->Listen found")
	}
	if v, exist := common.Cfg.GetBoolProperty("LocalServer", "AutoOpenWebUI"); !exist || v {
		go func() {
			time.Sleep(1 * time.Second)
			util.OpenBrowser("http://localhost:" + common.ProxyPort + "/")
		}()
	}

	startLocalProxyServer(addr, proxy.GLOBAL_PROXY_SERVER)
	//launchSystemTray()

}
