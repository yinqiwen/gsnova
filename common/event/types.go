package event

const (
	EventHttpReq         = 10000
	EventHttpRes         = 10001
	EventTCPOpen         = 10002
	EventConnClose       = 10003
	EventTCPChunk        = 10004
	EventAuth            = 10005
	EventNotify          = 10006
	EventUDP             = 10008
	EventHeartBeat       = 10009
	EventChannelCloseReq = 10010
	EventChannelCloseACK = 10011
	EventPortUnicast     = 10012
	EventConnTest        = 10013

	NoneCompressor    = 0
	SnappyCompressor  = 1
	NoneEncrypter     = 0
	RC4Encrypter      = 1
	Salsa20Encrypter  = 2
	AES256Encrypter   = 3
	Chacha20Encrypter = 4
	BlowfishEncrypter = 5
	//Chacha20Encypter = 3
)

func init() {
	RegistObject(EventHttpReq, &HTTPRequestEvent{})
	RegistObject(EventHttpRes, &HTTPResponseEvent{})
	RegistObject(EventTCPOpen, &TCPOpenEvent{})
	RegistObject(EventConnClose, &ConnCloseEvent{})
	RegistObject(EventTCPChunk, &TCPChunkEvent{})
	RegistObject(EventNotify, &NotifyEvent{})
	RegistObject(EventAuth, &AuthEvent{})
	RegistObject(EventUDP, &UDPEvent{})
	RegistObject(EventHeartBeat, &HeartBeatEvent{})
	RegistObject(EventChannelCloseReq, &ChannelCloseReqEvent{})
	RegistObject(EventChannelCloseACK, &ChannelCloseACKEvent{})
	RegistObject(EventPortUnicast, &PortUnicastEvent{})
	RegistObject(EventConnTest, &ConnTestEvent{})
}
