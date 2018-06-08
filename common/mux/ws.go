package mux

import (
	"io"
	"net"

	"github.com/gorilla/websocket"
	"github.com/yinqiwen/gsnova/common/logger"
)

type WsConn struct {
	*websocket.Conn
	msgReader io.Reader
}

func (ws *WsConn) writeBuffers(bufs *net.Buffers) (n int64, err error) {
	w, err := ws.NextWriter(websocket.BinaryMessage)
	if nil != err {
		return 0, err
	}
	defer w.Close()
	for _, b := range *bufs {
		nn, err := w.Write(b)
		if nil != err {
			return n, err
		}
		n += int64(nn)
	}

	return n, nil
}

func (ws *WsConn) Write(p []byte) (int, error) {
	c := ws.Conn
	if nil == c {
		return 0, io.EOF
	}
	err := c.WriteMessage(websocket.BinaryMessage, p)
	if nil != err {
		logger.Error("Failed to write websocket binary messgage:%v", err)
		return 0, err
	}
	return len(p), nil
}

func (ws *WsConn) readData(p []byte) (int, error) {
	if nil == ws.msgReader {
		return 0, io.EOF
	}
	n, err := ws.msgReader.Read(p)
	if nil != err {
		ws.msgReader = nil
	}
	return n, nil
}

func (ws *WsConn) Read(p []byte) (int, error) {
	if nil != ws.msgReader {
		return ws.readData(p)
	}
	if nil == ws.Conn {
		return 0, io.EOF
	}
	mt, reader, err := ws.Conn.NextReader()
	//mt, data, err := c.ReadMessage()
	if err != nil {
		return 0, err
	}
	switch mt {
	case websocket.BinaryMessage:
		ws.msgReader = reader
		return ws.readData(p)
	default:
		logger.Error("Invalid websocket message type:%d", mt)
		return 0, io.EOF
	}

}
