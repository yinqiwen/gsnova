package util

import (
	"fmt"
	//"net"
	"os"
	"testing"
)

func TestNet(t *testing.T) {

	v := IPv42Int("10.10.10.10")
	ip := Long2IPv4(v)
	if ip != "10.10.10.10" {
		fmt.Printf("%s  %d\n", ip, v)
		t.Error("Failed to conv ip to int:")
	}
	t.Error("####")
}

func TestLocalIP(t *testing.T) {
	t.Error("####" + GetLocalIP())
}

func TestIni(t *testing.T) {
	ini, err := LoadIniFile("snova.conf")
	if nil != err {
		t.Error("Failed to load ini file for reason:" + err.Error())
		return
	}
	file, err := os.OpenFile("snova_copy.ini", os.O_CREATE, os.ModePerm)
	if nil != err {
		t.Error("Failed to load write file for reason:" + err.Error())
		return
	}
	//s,ok := ini.GetProperty("Framework", "LocalPort")
	//t.Error("#######",s, ok)
	ini.Save(file)
	file.Close()
}
