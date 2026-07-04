package model

import "time"

type Protocol string

const (
	ProtocolRTSP   Protocol = "rtsp"
	ProtocolRTPUDP Protocol = "rtp-udp"
)

type Codec string

type MediaKind string

const (
	CodecH264    Codec = "h264"
	CodecH265    Codec = "h265"
	CodecMJPEG   Codec = "mjpeg"
	CodecUnknown Codec = "unknown"
)

const (
	MediaVideo   MediaKind = "video"
	MediaAudio   MediaKind = "audio"
	MediaUnknown MediaKind = "unknown"
)

type SessionBundle struct {
	Name          string
	Description   string
	Transport     []Protocol
	Streams       []Stream
	Timeline      []PacketEvent
	Script        *RTSPScript
	Metadata      map[string]string
	CaptureOffset time.Duration // offset applied to capture timestamps during replay (microsecond precision)
}

type Stream struct {
	ID          string
	Kind        MediaKind
	Codec       Codec
	ClockRate   int
	PayloadType uint8
	SSRC        uint32
	Parameters  map[string]string
}

type PacketEvent struct {
	StreamID   string
	Sequence   uint16
	Timestamp  uint32
	Marker     bool
	Payload    []byte
	EmittedAt  time.Duration
	ReceivedAt time.Duration
	Metadata   map[string]string
}

type RTSPScript struct {
	Exchanges []RTSPExchange
}

type RTSPExchange struct {
	Method         string
	Path           string
	Headers        map[string]string
	ExpectedStatus int
	Body           []byte
}
