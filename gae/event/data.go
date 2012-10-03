package event

import (
	"bytes"
	"codec"
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

func (cfg *GAEServerConfig)IsContentTypeInCompressFilter(v string)bool{
   for key := range cfg.CompressFilter {
		if strings.Index(v, key) != -1 {
		   return true
		}
	}
	return false
}

func (cfg *GAEServerConfig) Encode(buffer *bytes.Buffer) bool {
	codec.WriteUvarint(buffer, uint64(cfg.RetryFetchCount))
	codec.WriteUvarint(buffer, uint64(cfg.MaxXMPPDataPackageSize))
	codec.WriteUvarint(buffer, uint64(cfg.RangeFetchLimit))
	codec.WriteUvarint(buffer, uint64(cfg.CompressType))
	codec.WriteUvarint(buffer, uint64(cfg.EncryptType))
	buffer.WriteByte(cfg.IsMaster)
	codec.WriteUvarint(buffer, uint64(len(cfg.CompressFilter)))
	for key := range cfg.CompressFilter {
		codec.WriteVarString(buffer, key)
	}
	return true
}

func (cfg *GAEServerConfig) Decode(buffer *bytes.Buffer) bool {
	tmp1, err1 := codec.ReadUvarint(buffer)
	tmp2, err2 := codec.ReadUvarint(buffer)
	tmp3, err3 := codec.ReadUvarint(buffer)
	tmp4, err4 := codec.ReadUvarint(buffer)
	tmp5, err5 := codec.ReadUvarint(buffer)
	tmp7, err7 := buffer.ReadByte()
	tmp6, err6 := codec.ReadUvarint(buffer)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil || err7 != nil {
		return false
	}
	cfg.RetryFetchCount = uint32(tmp1)
	cfg.MaxXMPPDataPackageSize = uint32(tmp2)
	cfg.RangeFetchLimit = uint32(tmp3)
	cfg.CompressType = uint32(tmp4)
	cfg.EncryptType = uint32(tmp5)
	cfg.IsMaster = uint8(tmp7)
	filter := make(map[string]string)
	for i := 0; i < int(tmp6); i++ {
		line, ok := codec.ReadVarString(buffer)
		if !ok {
			return false
		}
		filter[line] = line
	}
	cfg.CompressFilter = filter
	return true
}

type User struct {
	Email     string
	Passwd    string
	Group     string
	AuthToken string
	BlackList map[string]string
}

func (cfg *User) Encode(buffer *bytes.Buffer) bool {
	codec.WriteVarString(buffer, cfg.Email)
	codec.WriteVarString(buffer, cfg.Passwd)
	codec.WriteVarString(buffer, cfg.Group)
	codec.WriteVarString(buffer, cfg.AuthToken)
	codec.WriteUvarint(buffer, uint64(len(cfg.BlackList)))
	for key := range cfg.BlackList {
		codec.WriteVarString(buffer, key)
	}
	return true
}
func (cfg *User) Decode(buffer *bytes.Buffer) bool {
	var ok bool
	if cfg.Email, ok = codec.ReadVarString(buffer); !ok {
		return false
	}
	if cfg.Passwd, ok = codec.ReadVarString(buffer); !ok {
		return false
	}
	if cfg.Group, ok = codec.ReadVarString(buffer); !ok {
		return false
	}
	if cfg.AuthToken, ok = codec.ReadVarString(buffer); !ok {
		return false
	}
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	blacklist := make(map[string]string)
	for i := 0; i < int(tmp); i++ {
		line, success := codec.ReadVarString(buffer)
		if !success {
			return false
		}
		blacklist[line] = line
	}
	cfg.BlackList = blacklist
	return true
}

type Group struct {
	Name      string
	BlackList map[string]string
}

func (cfg *Group) Encode(buffer *bytes.Buffer) bool {
	codec.WriteVarString(buffer, cfg.Name)
	codec.WriteUvarint(buffer, uint64(len(cfg.BlackList)))
	for key := range cfg.BlackList {
		codec.WriteVarString(buffer, key)
	}
	return true
}
func (cfg *Group) Decode(buffer *bytes.Buffer) bool {
	var ok bool
	if cfg.Name, ok = codec.ReadVarString(buffer); !ok {
		return false
	}
	tmp, err := codec.ReadUvarint(buffer)
	if err != nil {
		return false
	}
	blacklist := make(map[string]string)
	for i := 0; i < int(tmp); i++ {
		line, success := codec.ReadVarString(buffer)
		if !success {
			return false
		}
		blacklist[line] = line
	}
	cfg.BlackList = blacklist
	return true
}
