package event

import (
	"bytes"
	"strings"
)

type GAEServerConfig struct {
	RetryFetchCount        uint32
	MaxXMPPDataPackageSize uint32
	RangeFetchLimit        uint32
	CompressType           uint32
	EncryptType            uint32
	IsMaster               uint8
	CompressFilter         map[string]string
}

func (cfg *GAEServerConfig) IsContentTypeInCompressFilter(v string) bool {
	for key := range cfg.CompressFilter {
		if strings.Index(v, key) != -1 {
			return true
		}
	}
	return false
}

func (cfg *GAEServerConfig) Encode(buffer *bytes.Buffer) {
	//	EncodeUInt32Value(buffer, cfg.RetryFetchCount)
	//	EncodeUInt32Value(buffer, cfg.MaxXMPPDataPackageSize)
	//	EncodeUInt32Value(buffer, cfg.RangeFetchLimit)
	//	EncodeUInt32Value(buffer, cfg.CompressType)
	//	EncodeUInt32Value(buffer, cfg.EncryptType)
	//	buffer.WriteByte(cfg.IsMaster)
	//	EncodeUInt32Value(buffer, uint32(len(cfg.CompressFilter)))
	//	for key := range cfg.CompressFilter {
	//		codec.WriteVarString(buffer, key)
	//	}
}

func (cfg *GAEServerConfig) Decode(buffer *bytes.Buffer) (err error) {
	return nil
}

type User struct {
	Email     string
	Passwd    string
	Group     string
	AuthToken string
	BlackList map[string]string
}

func (cfg *User) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, cfg.Email)
	EncodeStringValue(buffer, cfg.Passwd)
	EncodeStringValue(buffer, cfg.Group)
	EncodeStringValue(buffer, cfg.AuthToken)
	EncodeUInt64Value(buffer, uint64(len(cfg.BlackList)))
	for key := range cfg.BlackList {
		EncodeStringValue(buffer, key)
	}
}
func (cfg *User) Decode(buffer *bytes.Buffer) (err error) {
	if cfg.Email, err = DecodeStringValue(buffer); nil != err {
		return
	}
	if cfg.Passwd, err = DecodeStringValue(buffer); nil != err {
		return
	}
	if cfg.Group, err = DecodeStringValue(buffer); nil != err {
		return
	}
	if cfg.AuthToken, err = DecodeStringValue(buffer); nil != err {
		return
	}
	var tmp uint32
	tmp, err = DecodeUInt32Value(buffer)
	if err != nil {
		return err
	}
	blacklist := make(map[string]string)
	for i := 0; i < int(tmp); i++ {
	    var line string
		if line, err = DecodeStringValue(buffer); nil != err{
		  return
		}
		blacklist[line] = line
	}
	cfg.BlackList = blacklist
	return nil
}

type Group struct {
	Name      string
	BlackList map[string]string
}

func (cfg *Group) Encode(buffer *bytes.Buffer) {
	EncodeStringValue(buffer, cfg.Name)
	EncodeUInt64Value(buffer, uint64(len(cfg.BlackList)))
	for key := range cfg.BlackList {
		EncodeStringValue(buffer, key)
	}
}
func (cfg *Group) Decode(buffer *bytes.Buffer) (err error) {
	if cfg.Name, err = DecodeStringValue(buffer); nil != err {
		return 
	}
	var tmp uint32
	tmp, err = DecodeUInt32Value(buffer)
	if err != nil {
		return 
	}
	blacklist := make(map[string]string)
	for i := 0; i < int(tmp); i++ {
		var line string
		if line, err = DecodeStringValue(buffer); nil != err{
		  return
		}
		blacklist[line] = line
	}
	cfg.BlackList = blacklist
	return nil
}
