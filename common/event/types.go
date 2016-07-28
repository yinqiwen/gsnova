package event

const (
	EventHttpReq   = 10000
	EventHttpRes   = 10001
	EventTCPOpen   = 10002
	EventTCPClose  = 10003
	EventTCPChunk  = 10004
	EventAuth      = 10005
	EventError     = 10006
	EventIVSetting = 10007
	EventUDP       = 10008

	NoneCompressor   = 0
	SnappyCompressor = 1
	NoneEncypter     = 0
	WithEncypter     = 1
)

func init() {
	RegistObject(EventHttpReq, &HTTPRequestEvent{})
	RegistObject(EventHttpRes, &HTTPResponseEvent{})
	RegistObject(EventTCPOpen, &TCPOpenEvent{})
	RegistObject(EventTCPClose, &TCPCloseEvent{})
	RegistObject(EventTCPChunk, &TCPChunkEvent{})
	RegistObject(EventError, &ErrorEvent{})
	RegistObject(EventAuth, &AuthEvent{})
	RegistObject(EventIVSetting, &IVSettingEvent{})
	RegistObject(EventUDP, &UDPEvent{})
}
