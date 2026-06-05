package model

func NewMinimalSessionBundle(name string, codec Codec, sourceKind string, location string, options map[string]string) *SessionBundle {
	metadata := map[string]string{
		"source.kind":     sourceKind,
		"source.location": location,
	}
	for key, value := range options {
		metadata["option."+key] = value
	}

	streamID := name + "-stream-0"
	return &SessionBundle{
		Name:        name,
		Description: "minimal metadata-only session bundle",
		Streams: []Stream{
			{
				ID:          streamID,
				Kind:        DefaultMediaKind(codec),
				Codec:       codec,
				ClockRate:   defaultClockRate(codec),
				PayloadType: defaultPayloadType(codec),
				SSRC:        1,
				Parameters:  map[string]string{},
			},
		},
		Timeline: []PacketEvent{
			{
				StreamID:  streamID,
				Sequence:  1,
				Timestamp: 0,
				Marker:    false,
				Payload:   []byte{0x00},
				Metadata:  map[string]string{"generated": "true"},
			},
		},
		Metadata: metadata,
	}
}

func defaultClockRate(codec Codec) int {
	switch codec {
	case CodecMJPEG:
		return 90000
	case CodecH264, CodecH265:
		return 90000
	default:
		return 90000
	}
}

func defaultPayloadType(codec Codec) uint8 {
	switch codec {
	case CodecMJPEG:
		return 26
	case CodecH264:
		return 96
	case CodecH265:
		return 98
	default:
		return 96
	}
}
