package event

const (
	EventHttpReq  = 10000
	EventHttpRes  = 10001
	EventTCPOpen  = 10002
	EventTCPClose = 10003
	EventTCPChunk = 10004
	EventAuth     = 10005
	EventNotify   = 10006
	EventUDP      = 10008
	EventLogin    = 10009

	NoneCompressor   = 0
	SnappyCompressor = 1
	NoneEncypter     = 0
	RC4Encypter      = 1
	Salsa20Encypter  = 2
	AES256Encypter   = 3
	//Chacha20Encypter = 3
)

func init() {
	RegistObject(EventHttpReq, &HTTPRequestEvent{})
	RegistObject(EventHttpRes, &HTTPResponseEvent{})
	RegistObject(EventTCPOpen, &TCPOpenEvent{})
	RegistObject(EventTCPClose, &TCPCloseEvent{})
	RegistObject(EventTCPChunk, &TCPChunkEvent{})
	RegistObject(EventNotify, &NotifyEvent{})
	RegistObject(EventAuth, &AuthEvent{})
	RegistObject(EventUDP, &UDPEvent{})
}
