package mp4

import (
	"encoding/binary"
	"fmt"
	"strings"
)

type ftypInfo struct {
	MajorBrand       string
	MinorVersion     uint32
	CompatibleBrands []string
}

func parseFTYP(header []byte) (*ftypInfo, error) {
	if len(header) < 16 {
		return nil, fmt.Errorf("ftyp header too short")
	}
	if string(header[4:8]) != "ftyp" {
		return nil, fmt.Errorf("missing ftyp box")
	}

	info := &ftypInfo{
		MajorBrand:   normalizeBrand(header[8:12]),
		MinorVersion: binary.BigEndian.Uint32(header[12:16]),
	}

	for i := 16; i+4 <= len(header); i += 4 {
		info.CompatibleBrands = append(info.CompatibleBrands, normalizeBrand(header[i:i+4]))
	}
	return info, nil
}

func normalizeBrand(value []byte) string {
	return strings.TrimRight(string(value), "\x00 ")
}
