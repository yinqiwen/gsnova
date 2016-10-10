package proxy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/yinqiwen/gsnova/common/helper"
)

var errIPRangeNotMatch = errors.New("No ip range could match the ip")

const cnIPFile = "apnic_cn.txt"

type IPRange struct {
	Start, End uint64
	Country    string
}

type IPRangeHolder struct {
	ranges []*IPRange
}

func (h *IPRangeHolder) Clear() {
	h.ranges = make([]*IPRange, 0)
}

func (h *IPRangeHolder) Len() int {
	return len(h.ranges)
}

// Less returns whether the element with index i should sort
// before the element with index j.
func (h *IPRangeHolder) Less(i, j int) bool {
	// if h.ranges[i].Start != h.ranges[j].Start {
	// 	return h.ranges[i].Start < h.ranges[j].Start
	// }
	return h.ranges[i].Start < h.ranges[j].Start
}

// Swap swaps the elements with indexes i and j.
func (h *IPRangeHolder) Swap(i, j int) {
	tmp := h.ranges[i]
	h.ranges[i] = h.ranges[j]
	h.ranges[j] = tmp
}

func (h *IPRangeHolder) sort() {
	sort.Sort(h)
}

func (h *IPRangeHolder) FindCountry(ip string) (string, error) {
	v, err := helper.IPv42Int(ip)
	if nil != err {
		log.Printf("Failed to convert ip to int for reason:%v", err)
		return "z2", err
	}

	compare := func(i int) bool {
		return h.ranges[i].Start >= uint64(v)
	}
	index := sort.Search(len(h.ranges), compare)
	if index == len(h.ranges) {
		//log.Printf("####%d\n", len(h.ranges))
		return "zz", errIPRangeNotMatch
	}
	if index > 0 {
		if h.ranges[index].Start == uint64(v) && h.ranges[index].End >= uint64(v) {
			log.Printf("%s match ip range %s-%s", ip, helper.Long2IPv4(h.ranges[index].Start), helper.Long2IPv4(h.ranges[index].End))
			return h.ranges[index].Country, nil
		}
		if index > 0 {
			if h.ranges[index-1].Start <= uint64(v) && h.ranges[index-1].End >= uint64(v) {
				log.Printf("%s match ip range %s-%s", ip, helper.Long2IPv4(h.ranges[index-1].Start), helper.Long2IPv4(h.ranges[index-1].End))
				return h.ranges[index-1].Country, nil
			}
		}
		// if h.ranges[index].Start > uint64(v) || h.ranges[index].End < uint64(v) {
		// 	log.Printf("Got start:%d - %d for %d", h.ranges[index].Start, h.ranges[index].End, v)
		// 	return "z1", errIPRangeNotMatch
		// }
		// return h.ranges[index].Country, nil
	}
	return "", errIPRangeNotMatch
}

func parseApnicIPReader(rc io.ReadCloser, persist bool) (*IPRangeHolder, error) {
	var err error
	reader := bufio.NewReader(rc)
	var buffer bytes.Buffer
	var (
		part   []byte
		prefix bool
	)
	defer rc.Close()
	holder := new(IPRangeHolder)
	var perststFile *os.File
	if persist {
		var err error
		perststFile, err = os.Create(proxyHome + "/" + cnIPFile)
		if nil == err {
			defer perststFile.Close()
		} else {
			log.Printf("[ERROR]Failed to open CN IP file:%v", err)
		}
	}
	for {
		if part, prefix, err = reader.ReadLine(); err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}
		buffer.Write(part)
		if !prefix {
			line := buffer.String()
			buffer.Reset()
			if strings.HasPrefix(line, "#") {
				continue
			}
			sp := strings.Split(line, "|")
			if len(sp) >= 6 {
				if sp[1] == "CN" && sp[2] == "ipv4" {
					startip, _ := helper.IPv42Int(sp[3])
					ipcount, _ := strconv.ParseUint(sp[4], 10, 32)
					tmp := &IPRange{uint64(startip), uint64(startip) + uint64(ipcount-1), sp[1]}
					holder.ranges = append(holder.ranges, tmp)
					if nil != perststFile {
						perststFile.Write([]byte(line))
						perststFile.Write([]byte("\r\n"))
					}
				}
			}
		}
	}
	holder.sort()
	return holder, nil
}

func parseApnicIPFile(name string) (*IPRangeHolder, error) {
	var file *os.File
	var err error
	if file, err = os.Open(name); err != nil {
		return nil, err
	}
	return parseApnicIPReader(file, false)
}

func getCNIPRangeHolder(hc *http.Client) (*IPRangeHolder, error) {
	url := "http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest"
	res, err := hc.Get(url)
	if nil != err {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Invalid IP range response:%v", res)
	}
	return parseApnicIPReader(res.Body, true)
}
