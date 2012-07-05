package util

import (
	"os"
	"testing"
)

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
