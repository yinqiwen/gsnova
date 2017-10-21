package common

import (
	_ "github.com/yinqiwen/gsnova/common/channel/direct"
	_ "github.com/yinqiwen/gsnova/common/channel/http"
	_ "github.com/yinqiwen/gsnova/common/channel/http2"
	_ "github.com/yinqiwen/gsnova/common/channel/kcp"
	_ "github.com/yinqiwen/gsnova/common/channel/quic"
	_ "github.com/yinqiwen/gsnova/common/channel/ssh"
	_ "github.com/yinqiwen/gsnova/common/channel/tcp"
	_ "github.com/yinqiwen/gsnova/common/channel/websocket"
)
