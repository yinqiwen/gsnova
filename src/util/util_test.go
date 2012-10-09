package util

import (
	"fmt"
	//"net"
	"os"
	"testing"
)

func TestNet(t *testing.T) {

	v,_ := IPv42Int("1.0.4.0")
	ip := Long2IPv4(v)
	if ip != "1.0.4.0" {
		t.Error("Failed to conv ip to int:")
	}
	fmt.Printf("%s  %d\n", ip, v)
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
