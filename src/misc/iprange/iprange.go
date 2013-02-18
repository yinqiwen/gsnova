package iprange

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"util"
)

type IPRange struct {
	Start, End uint64
	Country    string
}

type IPRangeHolder struct {
	ranges []*IPRange
}

func (h *IPRangeHolder) Len() int {
	return len(h.ranges)
}

// Less returns whether the element with index i should sort
// before the element with index j.
func (h *IPRangeHolder) Less(i, j int) bool {
	if h.ranges[i].Start != h.ranges[j].Start {
		return h.ranges[i].Start < h.ranges[j].Start
	}
	return h.ranges[i].End < h.ranges[j].End
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
	v, err := util.IPv42Int(ip)
	if nil != err {
		log.Printf("Failed to convert ip to int for reason:%v\n", err)
		return "", err
	}

	compare := func(i int) bool {
		return h.ranges[i].Start > uint64(v) || h.ranges[i].Start <= uint64(v) && h.ranges[i].End >= uint64(v)
	}
	index := sort.Search(len(h.ranges), compare)
	if index == len(h.ranges) {
		return "", nil
	}
	if index > 0 {
		if h.ranges[index].Start > uint64(v) || h.ranges[index].End < uint64(v) {
			return "", nil
		}
		return h.ranges[index].Country, nil
	}
	return "", fmt.Errorf("No record found.")
}

func ParseApnic(name string) (*IPRangeHolder, error) {
	var file *os.File
	var err error
	if file, err = os.Open(name); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	var buffer bytes.Buffer
	var (
		part   []byte
		prefix bool
	)
	holder := new(IPRangeHolder)
	for {
		if part, prefix, err = reader.ReadLine(); err != nil {
			break
		}
		buffer.Write(part)
		if !prefix {
			line := buffer.String()
			buffer.Reset()
			sp := strings.Split(line, "|")
			if len(sp) >= 6 {
				if sp[1] == "CN" && sp[2] == "ipv4" {
					startip, _ := util.IPv42Int(sp[3])
					ipcount, _ := strconv.ParseUint(sp[4], 10, 32)
					tmp := &IPRange{uint64(startip), uint64(startip) + uint64(ipcount-1), sp[1]}
					holder.ranges = append(holder.ranges, tmp)
				}
			}
		}
	}
	file.Close()
	holder.sort()
	return holder, nil
}

func ParseWipmania(name string) (*IPRangeHolder, error) {
	// Open a zip archive for reading.
	file_name := "worldip.en.txt"
	r, err := zip.OpenReader(name)
	if err != nil {
		log.Printf("Failed to open ip range file for reason:%v", err)
		return nil, err
	}
	defer r.Close()

	// Iterate through the files in the archive,
	// printing some of their contents.
	holder := new(IPRangeHolder)
	for _, f := range r.File {
		if f.Name != file_name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			log.Printf("Failed to open ip range file for reason:%v", err)
			return nil, err
		}
		reader := bufio.NewReader(rc)
		var buffer bytes.Buffer
		var (
			part   []byte
			prefix bool
		)
		for {
			if part, prefix, err = reader.ReadLine(); err != nil {
				break
			}
			buffer.Write(part)
			if !prefix {
				line := buffer.String()
				buffer.Reset()
				sp := strings.Split(line, ",")
				if len(sp) >= 5 {
					start, _ := strconv.ParseUint(strings.Trim(sp[2], "\""), 10, 32)
					end, _ := strconv.ParseUint(strings.Trim(sp[3], "\""), 10, 32)
					country := strings.Trim(sp[4], "\"")
					if strings.EqualFold(country, "CN") {
						tmp := &IPRange{start, end, country}
						holder.ranges = append(holder.ranges, tmp)
					}
				}
			}
		}
		rc.Close()
		holder.sort()
	}
	return holder, nil
}
