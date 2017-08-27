package mux

import (
	"bytes"
	"io"
	"log"

	"github.com/gorilla/websocket"
)

type WsConn struct {
	*websocket.Conn
	readbuf bytes.Buffer
}

func (ws *WsConn) Write(p []byte) (int, error) {
	c := ws.Conn
	if nil == c {
		return 0, io.EOF
	}
	err := c.WriteMessage(websocket.BinaryMessage, p)
	if nil != err {
		log.Printf("Failed to write websocket binary messgage:%v", err)
		return 0, err
	}
	return len(p), nil
}

func (ws *WsConn) Read(p []byte) (int, error) {
	if nil == ws.Conn {
		return 0, io.EOF
	}
	if ws.readbuf.Len() > 0 {
		return ws.readbuf.Read(p)
	}
	ws.readbuf.Reset()
	c := ws.Conn
	if nil == c {
		return 0, io.EOF
	}
	mt, data, err := c.ReadMessage()
	if err != nil {
		return 0, err
	}
	switch mt {
	case websocket.BinaryMessage:
		ws.readbuf.Write(data)
		return ws.readbuf.Read(p)
	default:
		log.Printf("Invalid websocket message type")
		return 0, io.EOF
	}
}
