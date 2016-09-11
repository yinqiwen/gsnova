package main

import (
	"fmt"
	"io"
	"log"

	"github.com/yinqiwen/gotoolkit/ots"
	"github.com/yinqiwen/gsnova/remote"
)

func dumpServerStat(args []string, c io.Writer) error {
	fmt.Fprintf(c, "Version:    %s\n", remote.Version)
	fmt.Fprintf(c, "NumSession:    %d\n", remote.GetSessionTableSize())
	fmt.Fprintf(c, "NumEventQueue: %d\n", remote.GetEventQueueSize())
	fmt.Fprintf(c, "NumActiveDynamicServer: %d\n", activeDynamicServerSize())
	fmt.Fprintf(c, "NumRetiredDynamicServer: %d\n", retiredDynamicServerSize())
	fmt.Fprintf(c, "TotalUserConn: %d\n", totalConn)
	return nil
}
func dumpServerSession(args []string, c io.Writer) error {
	remote.DumpAllSession(c)
	return nil
}
func dumpServerQueue(args []string, c io.Writer) error {
	remote.DumpAllQueue(c)
	return nil
}

func main() {
	ots.RegisterHandler("vstat", dumpServerStat, 0, 0, "VStat                                 Dump server stat")
	ots.RegisterHandler("sls", dumpServerSession, 0, 0, "SLS                                  List server sessions")
	ots.RegisterHandler("qls", dumpServerQueue, 0, 0, "QLS                                  List server event queues")
	err := ots.StartTroubleShootingServer(remote.ServerConf.AdminListen)
	if nil != err {
		log.Printf("Failed to start admin server with reason:%v", err)
		return
	}
	startLocalProxyServer(remote.ServerConf.Listen)
}
