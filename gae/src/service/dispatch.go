package service

import (
	"appengine"
	"bytes"
	"fmt"
	"event"
)

func mergeSegmentEvents(ctx appengine.Context, ev *event.SegmentEvent) event.Event {
	//not supported now
	return nil
}

func handleRecvEvent(tags *event.EventHeaderTags, ev event.Event, ctx appengine.Context) (event.Event, error) {
	var res event.Event
	switch ev.GetType() {
	case event.HTTP_REQUEST_EVENT_TYPE:
		res = Fetch(ctx, ev.(*event.HTTPRequestEvent))
	case event.RESERVED_SEGMENT_EVENT_TYPE:
		merged := mergeSegmentEvents(ctx, ev.(*event.SegmentEvent))
		if nil != merged {
			res,_ = handleRecvEvent(tags, merged, ctx)
		}
		return nil,fmt.Errorf("No merged event")
	case event.AUTH_REQUEST_EVENT_TYPE:
		res = Auth(ctx, ev.(*event.AuthRequestEvent))
	case event.USER_OPERATION_EVENT_TYPE:
		res = HandlerUserEvent(ctx, tags, ev.(*event.UserOperationEvent))
	case event.GROUP_OPERATION_EVENT_TYPE:
		res = HandlerGroupEvent(ctx, tags, ev.(*event.GroupOperationEvent))
	case event.USER_LIST_REQUEST_EVENT_TYPE:
		res = HandlerUserListEvent(ctx, tags, ev.(*event.ListUserRequestEvent))
	case event.GROUOP_LIST_REQUEST_EVENT_TYPE:
		res = HandlerGroupListEvent(ctx, tags, ev.(*event.ListGroupRequestEvent))
	case event.BLACKLIST_OPERATION_EVENT_TYPE:
		res = HandlerBalcklistEvent(ctx, tags, ev.(*event.BlackListOperationEvent))
	case event.SERVER_CONFIG_EVENT_TYPE:
		res = HandlerConfigEvent(ctx, ev.(*event.ServerConfigEvent))
	case event.SHARE_APPID_EVENT_TYPE:
		res = HandleShareEvent(ctx, ev.(*event.ShareAppIDEvent))
	case event.REQUEST_SHARED_APPID_EVENT_TYPE:
		res = RetrieveAppIds(ctx)
	case event.EVENT_TCP_CHUNK_TYPE:
	    res = TunnelWrite(ctx, tags, ev.(*event.TCPChunkEvent))
	case event.EVENT_TCP_CONNECTION_TYPE:
	    res = TunnelSocketConnection(ctx, tags, ev.(*event.SocketConnectionEvent))
	case event.EVENT_SOCKET_CONNECT_WITH_DATA_TYPE:
	    res = TunnelConn(ctx, tags, ev.(*event.SocketConnectWithDataEvent))
	case event.EVENT_SOCKET_READ_TYPE:
	    res = TunnelRead(ctx, tags, ev.(*event.SocketReadEvent))
	}
	return res, nil
}

func splitBuffer(buf *bytes.Buffer, hash uint32, size uint32, tags *event.EventHeaderTags) []*bytes.Buffer {
	var buflen uint32 = uint32(buf.Len())
	if buflen > size {
		total := buflen / size
		if buflen%size != 0 {
			total++
		}
		bufs := make([]*bytes.Buffer, total)
		var i uint32 = 0
		for ; i < total; i++ {
			seg := new(event.SegmentEvent)
			seg.SetHash(hash)
			seg.Sequence = i
			seg.Total = total
			blen := size
			buflen = uint32(buf.Len())
			if blen > buflen {
				blen = buflen
			}
			b := make([]byte, blen)
			buf.Read(b)
			seg.Content = b

			tmpbuf := new(bytes.Buffer)
			tags.Encode(tmpbuf)
			seg.Encode(tmpbuf)
			bufs[i] = tmpbuf
		}
		return bufs
	}
	return []*bytes.Buffer{buf}
}

func HandleEvent(tags *event.EventHeaderTags, ev event.Event, ctx appengine.Context, sender EventSendService) error {
	res, err := handleRecvEvent(tags, ev, ctx)
	if nil != err {
		ctx.Errorf("Failed to handle event[%d:%d] for reason:%v", ev.GetType(), ev.GetVersion(), err)
		return err
	}
	if nil == res{
	   var empty bytes.Buffer
	   sender.Send(&empty)
	   return nil
	}
	res.SetHash(ev.GetHash())
	compressType := ServerConfig.CompressType
	if httpres, ok := res.(*event.HTTPResponseEvent); ok {
		v := httpres.GetHeader("Content-Type")
		if len(v) > 0 {
			if ServerConfig.IsContentTypeInCompressFilter(v) {
				compressType = event.COMPRESSOR_NONE
			}
		}
	}
	x := new(event.CompressEvent)
	x.SetHash(ev.GetHash())
	x.CompressType = compressType
	x.Ev = res
	y := new(event.EncryptEvent)
	y.SetHash(ev.GetHash())
	y.EncryptType = ServerConfig.EncryptType
	y.Ev = x
	var buf bytes.Buffer
	tags.Encode(&buf)
	event.EncodeEvent(&buf, y)
	if sender.GetMaxDataPackageSize() > 0 && buf.Len() > sender.GetMaxDataPackageSize() {
		bufs := splitBuffer(&buf, uint32(ev.GetHash()), uint32(sender.GetMaxDataPackageSize()), tags)
		for _, x := range bufs {
			sender.Send(x)
		}
	} else {
		sender.Send(&buf)
	}
	return nil
}
