package rtpudp

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"stream-source-tester/internal/model"
)

// sendTCPInterleaved sends RTP packets over a raw TCP connection using the
// RTSP interleaved format: $ (0x24) + channel byte + 2-byte big-endian length + RTP payload.
// It loops continuously, repeating the timeline until the context is cancelled.
func sendTCPInterleaved(ctx context.Context, conn net.Conn, bundle *model.SessionBundle, rtpChannel byte) {
	if len(bundle.Streams) == 0 || len(bundle.Timeline) == 0 {
		return
	}

	streamIndex := make(map[string]model.Stream, len(bundle.Streams))
	for _, stream := range bundle.Streams {
		streamIndex[stream.ID] = stream
	}

	for {
		start := time.Now()

		for _, event := range bundle.Timeline {
			select {
			case <-ctx.Done():
				return
			default:
			}

			wait := event.EmittedAt - time.Since(start)
			if wait > 0 {
				time.Sleep(wait)
			}

			stream, ok := streamIndex[event.StreamID]
			if !ok {
				continue
			}

			packet := encodePacket(stream, event)
			interleaved := make([]byte, 4+len(packet))
			interleaved[0] = 0x24 // '$' magic byte for interleaved
			interleaved[1] = rtpChannel
			interleaved[2] = byte(len(packet) >> 8)
			interleaved[3] = byte(len(packet))
			copy(interleaved[4:], packet)

			_, err := conn.Write(interleaved)
			if err != nil {
				return
			}
		}

		// After one timeline loop, wait briefly before repeating
		if len(bundle.Timeline) > 0 {
			lastEmitted := bundle.Timeline[len(bundle.Timeline)-1].EmittedAt
			elapsed := time.Since(start)
			if remaining := lastEmitted - elapsed; remaining > 0 {
				time.Sleep(remaining)
			}
		}
	}
}

// parseTCPInterleavedTarget extracts host:port from a tcp://host:port URL.
func parseTCPInterleavedTarget(target string) (host, port string, ok bool) {
	// target is expected to be tcp://host:port or just host:port for UDP
	if !strings.HasPrefix(target, "tcp://") {
		return "", "", false
	}
	target = strings.TrimPrefix(target, "tcp://")
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return "", "", false
	}
	return host, port, true
}

// parseInterleavedChannel extracts the RTP channel byte from an interleaved option.
// Supports formats: "0-1" (uses first number), "0", or empty (defaults to 0).
func parseInterleavedChannel(interleavedOption string) byte {
	if interleavedOption == "" {
		return 0
	}
	// Handle "0-1" format
	if first, _, ok := strings.Cut(interleavedOption, "-"); ok {
		if n, err := strconv.Atoi(first); err == nil {
			return byte(n)
		}
	}
	// Handle plain number
	if n, err := strconv.Atoi(interleavedOption); err == nil {
		return byte(n)
	}
	return 0
}
