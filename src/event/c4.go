package event

import (
	"bytes"
)

type SequentialChunkEvent struct {
	Sequence uint32
	Content  []byte
	EventHeader
}

func (req *SequentialChunkEvent) Encode(buffer *bytes.Buffer) {
	EncodeUInt32Value(buffer, req.Sequence)
	EncodeBytesValue(buffer, req.Content)
}
func (req *SequentialChunkEvent) Decode(buffer *bytes.Buffer) (err error) {
	req.Sequence, err = DecodeUInt32Value(buffer)
	if err != nil {
		return
	}
	req.Content, err = DecodeBytesValue(buffer)
	if err != nil {
		return
	}
	return nil
}

func (req *SequentialChunkEvent) GetType() uint32 {
	return EVENT_SEQUNCEIAL_CHUNK_TYPE
}
func (req *SequentialChunkEvent) GetVersion() uint32 {
	return 1
}

type TransactionCompleteEvent struct {
	EventHeader
}

func (req *TransactionCompleteEvent) Encode(buffer *bytes.Buffer) {
}
func (req *TransactionCompleteEvent) Decode(buffer *bytes.Buffer) (err error) {
	return nil
}

func (req *TransactionCompleteEvent) GetType() uint32 {
	return EVENT_TRANSACTION_COMPLETE_TYPE
}
func (req *TransactionCompleteEvent) GetVersion() uint32 {
	return 1
}
