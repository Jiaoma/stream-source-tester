package pcap

import (
	"encoding/binary"
	"fmt"
)

type headerInfo struct {
	Magic        uint32
	Endian       string
	VersionMajor uint16
	VersionMinor uint16
	SnapLen      uint32
	LinkType     uint32
}

func parseHeader(header []byte) (*headerInfo, error) {
	if len(header) < 24 {
		return nil, fmt.Errorf("pcap header too short")
	}

	magicLE := binary.LittleEndian.Uint32(header[:4])
	magicBE := binary.BigEndian.Uint32(header[:4])

	var order binary.ByteOrder
	var endian string
	var magic uint32

	switch {
	case magicLE == 0xa1b2c3d4 || magicLE == 0xa1b23c4d:
		order = binary.LittleEndian
		endian = "little"
		magic = magicLE
	case magicBE == 0xa1b2c3d4 || magicBE == 0xa1b23c4d:
		order = binary.BigEndian
		endian = "big"
		magic = magicBE
	default:
		return nil, fmt.Errorf("unsupported pcap magic")
	}

	return &headerInfo{
		Magic:        magic,
		Endian:       endian,
		VersionMajor: order.Uint16(header[4:6]),
		VersionMinor: order.Uint16(header[6:8]),
		SnapLen:      order.Uint32(header[16:20]),
		LinkType:     order.Uint32(header[20:24]),
	}, nil
}
