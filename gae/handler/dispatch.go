package handler

import (
	"bytes"
	"appengine"
	"event"
	"service"
)

type DispatchEventHandler struct {

}

func mergeSegmentEvents(ctx appengine.Context, ev *event.SegmentEvent)event.Event{
   //not supported now
   return nil
}

func (dispatcher *DispatchEventHandler) handleRecvEvent(ctx appengine.Context, header *event.EventHeader, ev event.Event) event.Event {
	var res event.Event
	switch header.Type {
	case event.HTTP_REQUEST_EVENT_TYPE:
		res = service.Fetch(ctx, ev.(*event.HTTPRequestEvent))
	case event.RESERVED_SEGMENT_EVENT_TYPE:
	    merged := mergeSegmentEvents(ctx, ev.(*event.SegmentEvent))
	    merged.SetAttachement(ev.GetAttachement())
		if nil != merged{
		  var tmp = event.EventHeader{merged.GetType(),merged.GetVersion(), header.Hash}
		  res = dispatcher.handleRecvEvent(ctx, &tmp, merged)
		}
		return nil
	case event.COMPRESS_EVENT_TYPE:
		compressEvent := ev.(*event.CompressEvent)
		var tmp = event.EventHeader{compressEvent.Ev.GetType(), compressEvent.Ev.GetVersion(), header.Hash}
		compressEvent.Ev.SetAttachement(ev.GetAttachement())
		res = dispatcher.handleRecvEvent(ctx, &tmp, compressEvent.Ev)
	case event.ENCRYPT_EVENT_TYPE:
		encryptEvent := ev.(*event.EncryptEvent)
		var tmp = event.EventHeader{encryptEvent.Ev.GetType(), encryptEvent.Ev.GetVersion(), header.Hash}
		encryptEvent.Ev.SetAttachement(ev.GetAttachement())
		res = dispatcher.handleRecvEvent(ctx, &tmp, encryptEvent.Ev)
	case event.AUTH_REQUEST_EVENT_TYPE:
		res = service.Auth(ctx, ev.(*event.AuthRequestEvent))
	case event.USER_OPERATION_EVENT_TYPE:
		res = service.HandlerUserEvent(ctx, ev.(*event.UserOperationEvent))
	case event.GROUP_OPERATION_EVENT_TYPE:
		res = service.HandlerGroupEvent(ctx, ev.(*event.GroupOperationEvent))
	case event.USER_LIST_REQUEST_EVENT_TYPE:
		res = service.HandlerUserListEvent(ctx,ev.(*event.ListUserRequestEvent))
	case event.GROUOP_LIST_REQUEST_EVENT_TYPE:
		res = service.HandlerGroupListEvent(ctx, ev.(*event.ListGroupRequestEvent))
	case event.BLACKLIST_OPERATION_EVENT_TYPE:
		res = service.HandlerBalcklistEvent(ctx, ev.(*event.BlackListOperationEvent))
	case event.SERVER_CONFIG_EVENT_TYPE:
		res = service.HandlerConfigEvent(ctx,ev.(*event.ServerConfigEvent))
    case event.SHARE_APPID_EVENT_TYPE:
		res = service.HandleShareEvent(ctx,ev.(*event.ShareAppIDEvent))
	case event.REQUEST_SHARED_APPID_EVENT_TYPE:
		res = service.RetrieveAppIds(ctx)
	case event.REQUEST_ALL_SHARED_APPID_EVENT_TYPE:
	    res = service.RetrieveAllAppIds(ctx)
	}
	return res
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
			seg.Content.Write(b)

			tmpbuf := new(bytes.Buffer)
			event.EncodeEventWithTags(tmpbuf, seg, tags)
			bufs[i] = tmpbuf
		}
		return bufs
	}
	return []*bytes.Buffer{buf}
}

func (dispatcher *DispatchEventHandler) OnEvent(header *event.EventHeader, ev event.Event) {
	var ctx appengine.Context
	tags := ((ev.GetAttachement().([]interface{}))[0]).(*event.EventHeaderTags)
	ctx = ((ev.GetAttachement().([]interface{}))[1]).(appengine.Context)
	sendservice := ((ev.GetAttachement().([]interface{}))[2]).(service.EventSendService)
	//ctx.Infof("Received event hash :%d",  ev.GetHash())
	var res event.Event = dispatcher.handleRecvEvent(ctx, header, ev)
	if nil != res {
		res.SetHash(ev.GetHash())
		compressType := service.ServerConfig.CompressType
		if httpres, ok := res.(*event.HTTPResponseEvent); ok {
			v := httpres.GetHeader("Content-Type")
			if len(v) > 0 {
				if service.ServerConfig.IsContentTypeInCompressFilter(v) {
					compressType = event.C_NONE
				}
			}
		}
		x := new(event.CompressEvent)
		x.SetHash(ev.GetHash())
		x.CompressType = compressType
		x.Ev = res
		y := new(event.EncryptEvent)
		y.SetHash(ev.GetHash())
		y.EncryptType = service.ServerConfig.EncryptType
		y.Ev = x
		var buf bytes.Buffer
		event.EncodeEventWithTags(&buf, y, tags)
		if sendservice.GetMaxDataPackageSize() > 0 && buf.Len() > sendservice.GetMaxDataPackageSize() {
			bufs := splitBuffer(&buf, uint32(ev.GetHash()), uint32(sendservice.GetMaxDataPackageSize()), tags)
			//ctx.Infof("Buff total len %d while split len is %d",  buf.Len(), sendservice.GetMaxDataPackageSize())
			for _, x := range bufs {
		    	//ctx.Infof("Send  message with len %d",  x.Len())
				sendservice.Send(x)
			}
		} else {
			sendservice.Send(&buf)
		}
	} else {
		ctx.Errorf("Failed to handle event[%d:%d]", ev.GetType(), ev.GetVersion())
	}
}
