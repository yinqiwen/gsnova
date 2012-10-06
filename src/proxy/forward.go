package proxy

import (
	"bufio"
	"bytes"
	"common"
	"event"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"misc/socks"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"util"
)

type ForwardConnection struct {
	forward_conn         net.Conn
	conn_url             *url.URL
	proxyAddr            string
	forwardChan          chan int
	manager              *Forward
	closed               bool
	range_fetch_conns    []net.Conn
	range_fetch_raw_addr string
}

func (conn *ForwardConnection) Close() error {
	if nil != conn.forward_conn {
		conn.forward_conn.Close()
	}
	if nil != conn.forwardChan {
		close(conn.forwardChan)
		conn.forwardChan = nil
	}
	conn.closed = true
	if nil != conn.range_fetch_conns {
		for i, _ := range conn.range_fetch_conns {
			if nil != conn.range_fetch_conns[i] {
				conn.range_fetch_conns[i].Close()
			}
		}
		//conn.range_fetch_conns = nil
	}
	return nil
}

func createDirectForwardConn(hostport string) (net.Conn, error) {
	addr, _ := lookupReachableAddress(hostport)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if nil != err {
		conn, err = net.DialTimeout("tcp", addr, 4*time.Second)
	}
	if nil != err {
		expireBlockVerifyCache(addr)
	}
	return conn, err
}

func (conn *ForwardConnection) initForwardConn(proxyAddr string) error {
	if !strings.Contains(proxyAddr, ":") {
		proxyAddr = proxyAddr + ":80"
	}
	//	if strings.Contains(proxyAddr, "google.com") {
	//		log.Printf("############connect %s\n", proxyAddr)
	//	}
	if nil != conn.forward_conn && conn.proxyAddr == proxyAddr {
		return nil
	}
	//	if nil != conn.forward_conn {
	//		log.Printf("#######Close unmatched address conn %s %s", conn.proxyAddr, proxyAddr)
	//	}
	conn.Close()
	var err error
	conn.conn_url, err = url.Parse(conn.manager.target)
	if nil != err {
		return err
	}

	addr := conn.conn_url.Host
	if !conn.manager.overProxy {
		addr, _ = lookupReachableAddress(addr)
	}

	isSocks := strings.HasPrefix(strings.ToLower(conn.conn_url.Scheme), "socks")
	if !isSocks {
		conn.forward_conn, err = net.DialTimeout("tcp", addr, 2*time.Second)
		if nil != err {
			conn.forward_conn, err = net.DialTimeout("tcp", addr, 4*time.Second)
		}
		if nil != err {
			expireBlockVerifyCache(addr)
		}
	} else {
		proxy := &socks.Proxy{Addr: conn.conn_url.Host}
		if nil != conn.conn_url.User {
			proxy.Username = conn.conn_url.User.Username()
			proxy.Password, _ = conn.conn_url.User.Password()
		}
		conn.forward_conn, err = proxy.Dial("tcp", proxyAddr)
	}
	if nil != err {
		log.Printf("Failed to dial address:%s for reason:%s\n", addr, err.Error())
		return err
	} else {
		conn.proxyAddr = proxyAddr
	}
	if nil == conn.forwardChan {
		conn.forwardChan = make(chan int, 2)
	}
	conn.closed = false
	return nil
}

func (conn *ForwardConnection) GetConnectionManager() RemoteConnectionManager {
	return conn.manager
}

func (conn *ForwardConnection) writeHttpRequest(req *http.Request) error {
	var err error
	index := 0
	for {
		if conn.manager.overProxy {
			err = req.WriteProxy(conn.forward_conn)
		} else {
			err = req.Write(conn.forward_conn)
		}

		if nil != err {
			log.Printf("Resend request since error:%v occured.\n", err)
			conn.Close()
			conn.initForwardConn(req.Host)
		} else {
			return nil
		}
		index++
		if index == 2 {
			return err
		}
	}
	return nil
}

func (auto *ForwardConnection) rangeFetch(hash uint32, resp *http.Response, req *http.Request, originRange string, contentRange string, localConn net.Conn) error {
	resp.Header.Del("Content-Range")
	_, endpos, content_length := util.ParseContentRangeHeaderValue(contentRange)
	//length := content_length
	limit := content_length - 1
	if len(originRange) == 0 {
		resp.StatusCode = 200
		resp.Status = ""
		req.Header.Del("Range")
		resp.ContentLength = int64(content_length)
	} else {
		start, end := util.ParseRangeHeaderValue(originRange)
		if common.DebugEnable {
			log.Printf("Session[%d]Range %s while %d-%d\n", hash, originRange, start, end)
		}
		if end == -1 {
			resp.ContentLength = int64(content_length - start)
		} else {
			resp.ContentLength = int64(end - start + 1)
			limit = end
		}
		//length = int(resp.ContentLength)
		resp.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d\r\n", start, (int64(start)+resp.ContentLength-1), content_length))
	}
	resp.Header.Set("Connection", "keep-alive")
	resp.Close = false
	save_body := resp.Body
	var empty bytes.Buffer
	resp.Body = ioutil.NopCloser(&empty)
	resp.Write(localConn)
	if common.DebugEnable {
		var tmp bytes.Buffer
		resp.Write(&tmp)
		log.Printf("Session[%d]Range resonse is %s\n", hash, tmp.String())
	}

	rangeFetchChannel := make(chan *rangeChunk, 10)
	if nil == auto.range_fetch_conns {
		auto.range_fetch_conns = make([]net.Conn, hostRangeConcurrentFether)
	}
	log.Printf("Session[%d]Start %d range chunk %s from %s\n", hash, hostRangeConcurrentFether, contentRange, req.Host)
	for i := 0; i < int(hostRangeConcurrentFether); i++ {
		go auto.rangeFetchWorker(req, hash, i, endpos+1+i*int(hostRangeFetchLimitSize), limit, rangeFetchChannel)
	}
	if nil != save_body {
		io.Copy(localConn, save_body)
	}

	responsedChunks := make(map[int]*rangeChunk)
	stopedWorker := uint32(0)
	expectedPos := endpos + 1
	for {
		select {
		case chunk := <-rangeFetchChannel:
			if nil != chunk {
				if chunk.start < 0 {
					stopedWorker = stopedWorker + 1
				} else {
					responsedChunks[chunk.start] = chunk
				}
			}
		}
		for {
			if chunk, exist := responsedChunks[expectedPos]; exist {
				_, err := localConn.Write(chunk.content)
				delete(responsedChunks, expectedPos)
				if nil != err {
					log.Printf("????????????????????????????%v\n", err)
					auto.Close()
					//return err
				} else {
					expectedPos = expectedPos + len(chunk.content)
				}
			} else {
				//log.Printf("Session[%d]Expected %d\n", hash, expectedPos)
				break
			}
		}
		if stopedWorker >= hostRangeConcurrentFether {
			if len(responsedChunks) > 0 {
				log.Printf("Session[%d]Rest %d unwrite chunks.\n", hash, len(responsedChunks))
			}
			break
		}
	}
	//localConn.Close()
	return nil
}

func (auto *ForwardConnection) rangeFetchWorker(req *http.Request, hash uint32, index, startpos, limit int, ch chan *rangeChunk) error {
	var buf bytes.Buffer
	req.Write(&buf)
	clonereq, _ := http.ReadRequest(bufio.NewReader(&buf))
	//log.Printf("Session[%d]Fetch[%d] started with %d-%d to %s", hash, index, startpos, limit, req.Host)
	clonereq.Header.Set("Connection", "keep-alive")
	clonereq.Close = false
	rawaddr, _ := lookupReachableAddress(net.JoinHostPort(req.Host, "80"))
	if auto.range_fetch_raw_addr != rawaddr {
		if common.DebugEnable {
			log.Printf("##########Session[%d:%d]addr is %s-%s %p\n", hash, index, auto.range_fetch_raw_addr, rawaddr, auto)
		}
		auto.range_fetch_raw_addr = rawaddr
		if nil != auto.range_fetch_conns[index] {
			auto.range_fetch_conns[index].Close()
			auto.range_fetch_conns[index] = nil
		}
	}
	var err error
	getConn := func() (net.Conn, error) {
		if nil != auto.range_fetch_conns[index] {
			return auto.range_fetch_conns[index], nil
		}
		if common.DebugEnable {
			log.Printf("##########Session[%d:%d]Recreate conn for %s\n", hash, index, net.JoinHostPort(req.Host, "80"))
		}
		conn, err := createDirectForwardConn(net.JoinHostPort(req.Host, "80"))
		if nil != err {
			conn, err = createDirectForwardConn(net.JoinHostPort(req.Host, "80"))
		}
		auto.range_fetch_conns[index] = conn
		return conn, err
	}
	retry_count := 0
	retry_limit := 1
	for startpos < limit-1 && !auto.closed && retry_count <= retry_limit {
		endpos := startpos + int(hostRangeFetchLimitSize) - 1
		if endpos > limit {
			endpos = limit
		}

		_, err = getConn()
		if nil != err {
			break
		}
		clonereq.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", startpos, endpos))
		if needInjectCRLF(clonereq.Host) {
			auto.range_fetch_conns[index].Write([]byte("\r\n"))
		}

		err = clonereq.Write(auto.range_fetch_conns[index])
		if nil != err {
			log.Printf("Session[%d]Failed to fetch range chunk[%d:%d-%d]  for reason:%v %v", hash, startpos, endpos, limit, err)
			retry_count = retry_count + 1
			auto.range_fetch_conns[index].Close()
			auto.range_fetch_conns[index] = nil
			continue
		}
		resp, err := http.ReadResponse(bufio.NewReaderSize(auto.range_fetch_conns[index], 512*1024), clonereq)
		if nil != resp && resp.StatusCode == 302 {
			log.Printf("Session[%d] redirect to %s", hash, resp.Header.Get("Location"))
			clonereq.URL, _ = url.Parse(resp.Header.Get("Location"))
			auto.range_fetch_conns[index].Close()
			auto.range_fetch_conns[index] = nil
			continue
		}
		//try again
		if nil != err || resp.StatusCode > 206 {
			log.Printf("Session[%d]Failed to fetch range chunk[%d:%d-%d]  for reason:%v %v", hash, startpos, endpos, limit, err, resp)
			retry_count = retry_count + 1
			auto.range_fetch_conns[index].Close()
			auto.range_fetch_conns[index] = nil
			continue
		}
		chunk := &rangeChunk{}
		chunk.start = startpos
		chunk.content = make([]byte, resp.ContentLength)
		if n, er := io.ReadFull(resp.Body, chunk.content); nil != er || n != int(endpos-startpos+1) {
			log.Printf("[ERROR]Read rrror response %v with %d bytes for reason:%v\n", resp, n, er)
			retry_count = retry_count + 1
			auto.range_fetch_conns[index].Close()
			auto.range_fetch_conns[index] = nil
			continue
		}
		ch <- chunk
		log.Printf("Session[%d]Fetch[%d] fetched %d bytes chunk[%d:%d-%d]from %s.", hash, index, resp.ContentLength, startpos, endpos, limit, clonereq.Host)
		startpos = startpos + int(hostRangeFetchLimitSize*hostRangeConcurrentFether)
	}
	if nil != err {
		log.Printf("Session[%d]Fetch[%d] failed for %v", hash, index, err)
	}
	//end
	ch <- &rangeChunk{start: -1}
	return nil
}

func (auto *ForwardConnection) Request(conn *SessionConnection, ev event.Event) (err error, res event.Event) {
	f := func(local, remote net.Conn) {
		n, err := io.Copy(remote, local)
		if nil != err {
			//			if err != io.EOF {
			//				log.Printf("Failed to copy for reason:%s\n", err.Error())
			//			}
			local.Close()
			remote.Close()
		}
		auto.forwardChan <- int(n)
	}
	auto.closed = false
	switch ev.GetType() {
	case event.HTTP_REQUEST_EVENT_TYPE:
		req := ev.(*event.HTTPRequestEvent)
		addr := req.RawReq.Host
		if !strings.Contains(addr, ":") {
			addr = net.JoinHostPort(addr, "443")
		}
		err = auto.initForwardConn(addr)
		if nil == auto.forward_conn {
			log.Printf("Failed to connect forward proxy.\n")
			return err, nil
		}
		if conn.Type == HTTPS_TUNNEL {
			log.Printf("Session[%d]Request URL:%s %s\n", ev.GetHash(), req.RawReq.Method, req.RawReq.RequestURI)
			conn.LocalRawConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
			go f(conn.LocalRawConn, auto.forward_conn)
			go f(auto.forward_conn, conn.LocalRawConn)
			<-auto.forwardChan
			<-auto.forwardChan
			auto.Close()
			conn.State = STATE_SESSION_CLOSE
		} else {
			if strings.HasPrefix(req.RawReq.RequestURI, "http://") {
				log.Printf("Session[%d]Request URL:%s %s\n", ev.GetHash(), req.RawReq.Method, req.RawReq.RequestURI)
			} else {
				log.Printf("Session[%d]Request URL:%s %s%s\n", ev.GetHash(), req.RawReq.Method, req.RawReq.Host, req.RawReq.RequestURI)
			}
			if !auto.manager.overProxy && needInjectCRLF(req.RawReq.Host) {
				log.Printf("Session[%d]Inject CRLF for %s", ev.GetHash(), req.RawReq.Host)
				auto.forward_conn.Write([]byte("\r\n"))
			}
			rangeInjdected := false
			rangeHeader := req.RawReq.Header.Get("Range")
			req.RawReq.Header.Del("Proxy-Connection")
			//			ua := req.RawReq.Header.Get("User-Agent")
			norange_inject := false
			//			if strings.Contains(ua, "iPhone") {
			//				norange_inject = true
			//			}
			if !norange_inject && !auto.manager.overProxy && hostNeedInjectRange(req.RawReq.Host) {
				log.Printf("Session[%d]Inject Range for %s", ev.GetHash(), req.RawReq.Host)
				if len(rangeHeader) == 0 {
					req.RawReq.Header.Set("Range", fmt.Sprintf("bytes=0-%d", hostRangeFetchLimitSize-1))
					rangeInjdected = true
				} else {
					startPos, endPos := util.ParseRangeHeaderValue(rangeHeader)
					if common.DebugEnable {
						log.Printf("Session[%d]rangeHeader=%s while %d-%d  %v\n", ev.GetHash(), rangeHeader, startPos, endPos, req.RawReq.Header)
					}
					if endPos == -1 || endPos-startPos > int(hostRangeFetchLimitSize-1) {
						endPos = startPos + int(hostRangeFetchLimitSize-1)
						req.RawReq.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", startPos, endPos))
						rangeInjdected = true
					}
				}
			}

			err := auto.writeHttpRequest(req.RawReq)
			if nil != err {
				return err, nil
			}
			if common.DebugEnable {
				var tmp bytes.Buffer
				req.RawReq.Write(&tmp)
				log.Printf("Session[%d]Send request \n%s\n", ev.GetHash(), tmp.String())
			}
			resp, err := http.ReadResponse(bufio.NewReader(auto.forward_conn), req.RawReq)
			if err != nil {
				return err, nil
			}
			responsed := false
			if rangeInjdected {
				contentRange := resp.Header.Get("Content-Range")
				if len(contentRange) > 0 {
					auto.rangeFetch(ev.GetHash(), resp, req.RawReq, rangeHeader, contentRange, conn.LocalRawConn)
					responsed = true
				}
			}
			if !responsed {
				err = resp.Write(conn.LocalRawConn)
			}
			resp.Body.Close()
			if common.DebugEnable {
				var tmp bytes.Buffer
				resp.Write(&tmp)
				log.Printf("Session[%d]Recv response \n%s\n", ev.GetHash(), tmp.String())
			}
			if nil != err || resp.Close || req.RawReq.Close {

				conn.LocalRawConn.Close()
				auto.Close()
				conn.State = STATE_SESSION_CLOSE
			} else {
				conn.State = STATE_RECV_HTTP
			}
		}

	default:
	}
	return nil, nil
}

type Forward struct {
	target    string
	overProxy bool
}

func (manager *Forward) GetName() string {
	return FORWARD_NAME + manager.target
}

func (manager *Forward) GetArg() string {
	return manager.target
}
func (manager *Forward) RecycleRemoteConnection(conn RemoteConnection) {

}

func (manager *Forward) GetRemoteConnection(ev event.Event) (RemoteConnection, error) {
	g := new(ForwardConnection)
	g.manager = manager
	g.Close()
	return g, nil
}
