package rtsp

import (
	"net"
	"time"

	"stream-source-tester/internal/model"
)

func sendTimelineOnce(bundle *model.SessionBundle, target string) {
	if len(bundle.Streams) == 0 || len(bundle.Timeline) == 0 {
		return
	}
	addr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	stream := bundle.Streams[0]
	start := time.Now()
	for _, event := range bundle.Timeline {
		wait := event.EmittedAt - time.Since(start)
		if wait > 0 {
			time.Sleep(wait)
		}
		packet := encodeBridgePacket(stream, event)
		_, _ = conn.Write(packet)
	}
}

func encodeBridgePacket(stream model.Stream, event model.PacketEvent) []byte {
	payload := event.Payload
	packet := make([]byte, 12+len(payload))
	packet[0] = 0x80
	packet[1] = stream.PayloadType & 0x7f
	if event.Marker {
		packet[1] |= 0x80
	}
	packet[2] = byte(event.Sequence >> 8)
	packet[3] = byte(event.Sequence)
	packet[4] = byte(event.Timestamp >> 24)
	packet[5] = byte(event.Timestamp >> 16)
	packet[6] = byte(event.Timestamp >> 8)
	packet[7] = byte(event.Timestamp)
	packet[8] = byte(stream.SSRC >> 24)
	packet[9] = byte(stream.SSRC >> 16)
	packet[10] = byte(stream.SSRC >> 8)
	packet[11] = byte(stream.SSRC)
	copy(packet[12:], payload)
	return packet
}
