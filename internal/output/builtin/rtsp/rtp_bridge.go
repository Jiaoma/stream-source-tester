package rtsp

import (
	"context"
	"net"
	"time"

	"stream-source-tester/internal/model"
)

// sendTimeline sends timeline packets at their original timing. When loop is
// true it repeats continuously to simulate a live camera stream.
func sendTimeline(ctx context.Context, bundle *model.SessionBundle, target string, loop bool) {
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
	firstTimestamp := bundle.Timeline[0].Timestamp
	lastTimestamp := bundle.Timeline[len(bundle.Timeline)-1].Timestamp
	timestampStep := lastTimestamp - firstTimestamp + uint32(stream.ClockRate/30)
	if timestampStep == 0 {
		timestampStep = uint32(stream.ClockRate)
	}
	sequenceStep := uint16(len(bundle.Timeline))
	if sequenceStep == 0 {
		sequenceStep = 1
	}
	var loopIndex uint32

	for {
		start := time.Now()

		for _, event := range bundle.Timeline {
			// Check for cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

			wait := event.EmittedAt - time.Since(start)
			if wait > 0 {
				time.Sleep(wait)
			}

			outEvent := event
			outEvent.Timestamp = event.Timestamp + timestampStep*loopIndex
			outEvent.Sequence = event.Sequence + uint16(loopIndex)*sequenceStep
			packet := encodeBridgePacket(stream, outEvent)
			_, err := conn.Write(packet)
			if err != nil {
				return
			}
		}

		// After one loop through the timeline, wait briefly before repeating
		// to avoid busy-looping.
		// Find the last packet's EmittedAt to know total duration
		if len(bundle.Timeline) > 0 {
			lastEmitted := bundle.Timeline[len(bundle.Timeline)-1].EmittedAt
			elapsed := time.Since(start)
			if remaining := lastEmitted - elapsed; remaining > 0 {
				time.Sleep(remaining)
			}
		}
		if !loop {
			return
		}
		loopIndex++
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
