package util

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
)

type Ini struct {
	props map[string]map[string]string
}

func NewIni() *Ini {
	ini := new(Ini)
	ini.props = make(map[string]map[string]string)
	return ini
}

func (ini *Ini) Load(is io.Reader) (err error) {
	var (
		part   []byte
		prefix bool
	)
	reader := bufio.NewReader(is)
	currenttag := ""
	var buffer bytes.Buffer
	for {
		if part, prefix, err = reader.ReadLine(); err != nil {
			if err == io.EOF {
				err = nil
			}
			break
		}
		buffer.Write(part)
		if !prefix {
			line := buffer.String()
			buffer.Reset()
			line = strings.TrimSpace(line)
			if len(line) == 0 || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				currenttag = line[1 : len(line)-1]
			} else {
				idx := strings.Index(line, "=")
				if idx != -1 {
					key := strings.TrimSpace(line[0:idx])
					value := strings.TrimSpace(line[idx+1:])
					ini.SetProperty(currenttag, key, value)
				}
//				splits := strings.Split(line, "=")
//				if len(splits) >= 2 {
//					key := strings.TrimSpace(splits[0])
//					value := strings.TrimSpace(splits[1])
//					ini.SetProperty(currenttag, key, value)
//				}
			}
		}
	}
	return
}

func (ini *Ini) Save(os io.Writer) {
	if _, ok := ini.props[""]; ok {
		for k1, v1 := range ini.props[""] {
			line := k1 + " = " + v1 + "\r\n"
			os.Write([]byte(line))
		}
		os.Write([]byte("\r\n"))
	}
	for k, xm := range ini.props {
		if k != "" {
			k = "[" + k + "]\r\n"
			os.Write([]byte(k))
			for k1, v1 := range xm {
				line := k1 + " = " + v1 + "\r\n"
				os.Write([]byte(line))
			}
			os.Write([]byte("\r\n"))
		}
	}
}

func (ini *Ini) SetProperty(tag, key, value string) {
	if nil == ini.props[tag] {
		ini.props[tag] = make(map[string]string)
	}
	ini.props[tag][key] = value
}

func (ini *Ini) GetProperty(tag, key string) (value string, exist bool) {
	if m, ok := ini.props[tag]; !ok {
		exist = false
		return
	} else {
		value, exist = m[key]
	}
	return
}

func (ini *Ini) GetIntProperty(tag, key string) (value int64, exist bool) {
	var str string
	if str, exist = ini.GetProperty(tag, key); exist {
		v, err := strconv.ParseInt(str, 10, 64)
		if nil != err {
			exist = false
		}
		value = v
	}
	return
}

func (ini *Ini) GetBoolProperty(tag, key string) (value bool, exist bool) {
	var str string
	if str, exist = ini.GetProperty(tag, key); exist {
		if strings.EqualFold(str, "true") || strings.EqualFold(str, "1") {
			value = true
		} else {
			value = false
		}
	}
	return
}

func (ini *Ini) GetTagProperties(tag string) (map[string]string, bool) {
	v, exist := ini.props[tag]
	return v, exist
}

func LoadIniFile(path string) (ini *Ini, err error) {
	var file *os.File
	if file, err = os.Open(path); err != nil {
		return
	}
	ini = NewIni()
	err = ini.Load(file)
	return
}
