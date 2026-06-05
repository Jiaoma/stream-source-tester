package model

import "strings"

func ParseCodec(value string) Codec {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(CodecH264):
		return CodecH264
	case string(CodecH265):
		return CodecH265
	case string(CodecMJPEG):
		return CodecMJPEG
	default:
		return CodecUnknown
	}
}
