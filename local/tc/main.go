package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
)

func main() {
	c, err := net.Dial("tcp", "127.0.0.1:48100")
	if nil != err {
		fmt.Printf("Error:%v", err)
		return
	}
	req, err := http.NewRequest("GET", "http://www.baidu.com", nil)
	if nil != err {
		fmt.Printf("Error:%v", err)
		return
	}

	req.Write(c)
	var buf bytes.Buffer
	req.Write(&buf)
	fmt.Printf("%s\n", string(buf.Bytes()))
	for {
		b := make([]byte, 8192)
		n, err := c.Read(b)
		if nil != err {
			return
		}
		fmt.Printf("%s", string(b[0:n]))
	}
}
