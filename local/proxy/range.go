package proxy

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/yinqiwen/gsnova/common/event"
)

func rangeFetch(C *RemoteChannel, req *event.HTTPRequestEvent, begin, end int64) (*event.HTTPResponseEvent, error) {
	log.Printf("Session:%d range fetch %d-%d", req.GetId(), begin, end)
	rangeReq := new(event.HTTPRequestEvent)
	rangeReq.Headers = make(http.Header)
	for k, v := range req.Headers {
		rangeReq.Headers[k] = v
	}
	rangeReq.URL = req.URL
	rangeReq.Method = req.Method
	rangeReq.SetId(req.GetId())
	rangeReq.Headers.Set("Range", fmt.Sprintf("bytes=%d-%d", begin, end))
	ev, err := C.Request(rangeReq)
	if nil == err {
		res, ok := ev.(*event.HTTPResponseEvent)
		if ok {
			res.SetId(req.GetId())
			if res.StatusCode == 302 {
				location := res.Headers.Get("Location")
				log.Printf("Range fetch:%s redirect to %s", rangeReq.URL, location)
				rangeReq.URL = location
				return rangeFetch(C, rangeReq, begin, end)
			}
			if res.StatusCode < 400 {
				return res, nil
			}
		}
		err = fmt.Errorf("Invalid response:%d %v for range fetch", res.StatusCode, res.Headers)
	}
	log.Printf("[ERROR]Failed to range fetch[%d-%d] for %s", begin, end, req.URL)
	return nil, err
}

type rangeChunk struct {
	start   int64
	end     int64
	total   int64
	content []byte
}

func rangeResponseToChunk(res *event.HTTPResponseEvent) *rangeChunk {
	contentRange := res.Headers.Get("Content-Range")
	if len(contentRange) > 0 {
		chunk := &rangeChunk{}
		fmt.Sscanf(contentRange, "bytes %d-%d/%d", &chunk.start, &chunk.end, &chunk.total)
		chunk.content = res.Content
		return chunk
	} else {
		log.Printf("[ERROR]Invalid range response %d %v", res.StatusCode, res.Headers)
	}
	return nil
}

type RangeFetcher struct {
	SingleFetchLimit  int64
	ConcurrentFetcher int32
	C                 *RemoteChannel
}

func (f *RangeFetcher) Fetch(req *event.HTTPRequestEvent) (*event.HTTPResponseEvent, error) {
	firstRangeStart := int64(0)
	firstRangeEnd := f.SingleFetchLimit
	rangeHeader := req.Headers.Get("Range")
	total := int64(-1)
	allRangeEnd := int64(-1)
	if len(rangeHeader) > 0 {
		fmt.Sscanf(rangeHeader, "bytes=%d-%d", &firstRangeStart, &firstRangeEnd)
		total = firstRangeEnd - firstRangeStart + 1
		allRangeEnd = firstRangeEnd
		if total > f.SingleFetchLimit {
			firstRangeEnd = firstRangeStart + f.SingleFetchLimit - 1
		}
	}
	first, err := rangeFetch(f.C, req, firstRangeStart, firstRangeEnd)
	if nil != err {
		return nil, err
	}
	if first.StatusCode < 400 {
		return first, nil
	}
	firstChunk := rangeResponseToChunk(first)
	if nil == firstChunk {
		return nil, fmt.Errorf("Invalid first range response:%d %v", first.StatusCode, first.Headers)
	}
	if total < 0 {
		total = firstChunk.total
		allRangeEnd = firstChunk.total - 1
	}
	res := new(event.HTTPResponseEvent)
	res.SetId(req.GetId())
	if len(rangeHeader) > 0 {
		res.StatusCode = 206
	} else {
		res.StatusCode = 200
	}
	res.Headers = first.Headers
	res.Headers.Set("Content-Length", fmt.Sprintf("%d", total))
	res.Content = first.Content
	HandleEvent(res)
	if int64(len(res.Content)) == total {
		return nil, nil
	}
	conccurrentNum := int32(0)
	nextRangeStart := firstRangeEnd + 1
	chunkChannel := make(chan *rangeChunk, f.ConcurrentFetcher)
	chunkCache := make(map[int64]*rangeChunk)
	waitNextChunkStart := nextRangeStart
	stopFetch := false
	fetch := func(req *event.HTTPRequestEvent, begin, end int64) {
		defer atomic.AddInt32(&conccurrentNum, -1)
		res, err := rangeFetch(f.C, req, begin, end)
		if nil == err {
			chunk := rangeResponseToChunk(res)
			if nil != chunk {
				chunkChannel <- chunk
				return
			}
		}
		stopFetch = true
	}

	for !stopFetch && waitNextChunkStart <= allRangeEnd {
		for conccurrentNum < f.ConcurrentFetcher && nextRangeStart <= allRangeEnd && int32(len(chunkCache)) < 4*f.ConcurrentFetcher {
			end := nextRangeStart + f.SingleFetchLimit - 1
			if end >= allRangeEnd {
				end = allRangeEnd
			}
			nextRangeStart = allRangeEnd + 1
			atomic.AddInt32(&conccurrentNum, 1)
			go fetch(req, nextRangeStart, end)
		}
		select {
		case chunk := <-chunkChannel:
			if nil == chunk {
				stopFetch = true
				log.Printf("Nil chunk")
			} else {
				chunkCache[chunk.start] = chunk
				for _, chunk := range chunkCache {
					if waitNextChunkStart == chunk.start {
						chunkEvent := &event.TCPChunkEvent{}
						chunkEvent.SetId(req.GetId())
						chunkEvent.Content = chunk.content
						HandleEvent(chunkEvent)
						waitNextChunkStart = chunk.end + 1
						delete(chunkCache, chunk.start)
					}
				}
			}
		case <-time.After(10 * time.Millisecond):
		}
	}
	close(chunkChannel)
	log.Printf("Session:%d stop range fetch.", req.GetId())
	return nil, nil
}
