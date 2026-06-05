package model

func DefaultMediaKind(codec Codec) MediaKind {
	switch codec {
	case CodecH264, CodecH265, CodecMJPEG:
		return MediaVideo
	default:
		return MediaUnknown
	}
}

func DefaultClockRate(kind MediaKind, codec Codec) int {
	switch kind {
	case MediaAudio:
		return 48000
	case MediaVideo:
		return 90000
	default:
		return defaultClockRate(codec)
	}
}

func DefaultPayloadType(kind MediaKind, codec Codec, index int) uint8 {
	base := defaultPayloadType(codec)
	if kind == MediaAudio {
		base = 97
	}
	return base + uint8(index)
}
