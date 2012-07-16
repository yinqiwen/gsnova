package common

import (
	"util"
	"log"
)

var Cfg *util.Ini

func InitConfig() error {
	cfg, err := util.LoadIniFile(Home + "gsnova.conf")
	Cfg = cfg
	if nil != err {
	   log.Fatalf("Failed to load config file for reason:%s\n", err.Error())
	}
	err = util.LoadHostMapping(Home + "hosts.conf")
	return err
}
