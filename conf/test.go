package test
import (
	"bufio"
	"bytes"
	"common"
	"conf"
	"container/list"
	"crypto/tls"
	"crypto/x509"
	"event"
	"logging"
	"net"
	"net/http"
	"strconv"
	"sync"
)

func main() {
	path, err := filepath.Abs(os.Args[0])
	if nil != err {
		fmt.Println(err)
		return
	}
	bin, _ := filepath.Split(path)
	home, _ := filepath.Split(filepath.Clean(bin))
	common.SnovaHome = home
	conffile := home + string(filepath.Separator) + "conf" + string(filepath.Separator) + "snova.conf"
	conf.GlobalConf = conf.NewSnovaClientConf(conffile)
	if nil == conf.GlobalConf {
		common.Exit("Failed to load config file:" + conffile)
	}
	logging.InitLogger(home + string(filepath.Separator) + "log" + string(filepath.Separator) + "snova.log")
	server.StartLocalProxyServer(conf.GlobalConf.LocalListenServer)
	logging.Info("Local HTTP(S) server has started on " + conf.GlobalConf.LocalListenServer)
	common.Exit("")
}
