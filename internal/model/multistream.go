package model

import "time"

type MinimalStreamSpec struct {
	ID      string
	Kind    MediaKind
	Codec   Codec
	SSRC    uint32
	Payload []byte
}

func NewMinimalMultiStreamBundle(name string, sourceKind string, location string, options map[string]string, specs []MinimalStreamSpec) *SessionBundle {
	metadata := map[string]string{
		"source.kind":     sourceKind,
		"source.location": location,
	}
	for key, value := range options {
		metadata["option."+key] = value
	}

	streams := make([]Stream, 0, len(specs))
	timeline := make([]PacketEvent, 0, len(specs))
	for i, spec := range specs {
		streamID := spec.ID
		if streamID == "" {
			streamID = name + "-stream-" + string(rune('0'+i))
		}
		kind := spec.Kind
		if kind == "" {
			kind = DefaultMediaKind(spec.Codec)
		}
		streams = append(streams, Stream{
			ID:          streamID,
			Kind:        kind,
			Codec:       spec.Codec,
			ClockRate:   DefaultClockRate(kind, spec.Codec),
			PayloadType: DefaultPayloadType(kind, spec.Codec, i),
			SSRC:        spec.SSRC,
			Parameters:  map[string]string{},
		})
		payload := spec.Payload
		if len(payload) == 0 {
			payload = []byte{byte(i)}
		}
		timeline = append(timeline, PacketEvent{
			StreamID:   streamID,
			Sequence:   uint16(i + 1),
			Timestamp:  uint32(i * 1000),
			Marker:     false,
			Payload:    payload,
			EmittedAt:  time.Duration(i) * 20 * time.Millisecond,
			ReceivedAt: 0,
			Metadata:   map[string]string{"generated": "true"},
		})
	}

	return &SessionBundle{
		Name:        name,
		Description: "minimal multi-stream session bundle",
		Streams:     streams,
		Timeline:    timeline,
		Metadata:    metadata,
	}
}
