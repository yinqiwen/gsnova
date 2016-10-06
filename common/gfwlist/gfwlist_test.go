package gfwlist

import (
	"log"
	"net/http"
	"testing"
)

func TestGFWList(t *testing.T) {
	userRules := []string{"||4ter2n.com", "|https://85.17.73.31/"}
	gfwlist, err := NewGFWList("https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt", "", userRules, "gfwlist.txt", false)
	if nil != err {
		log.Printf("#####%v", err)
		return
	}
	req, _ := http.NewRequest("GET", "https://r2---sn-oguesnze.googlevideo.com", nil)
	v := gfwlist.IsBlockedByGFW(req)
	log.Printf("#####match %v", v)
}
