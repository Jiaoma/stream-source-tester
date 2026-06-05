package rtsp

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"stream-source-tester/internal/model"
)

func sendInterleavedOnce(conn net.Conn, bundle *model.SessionBundle, channel string) {
	if len(bundle.Streams) == 0 || len(bundle.Timeline) == 0 {
		return
	}
	channelID := byte(0)
	first, _, _ := strings.Cut(channel, "-")
	if parsed, err := strconv.Atoi(first); err == nil {
		channelID = byte(parsed)
	}

	stream := bundle.Streams[0]
	start := time.Now()
	for _, event := range bundle.Timeline {
		wait := event.EmittedAt - time.Since(start)
		if wait > 0 {
			time.Sleep(wait)
		}
		packet := encodeBridgePacket(stream, event)
		payload := []byte{0x24, channelID}
		length := len(packet)
		payload = append(payload, byte(length>>8), byte(length))
		payload = append(payload, packet...)
		_, _ = conn.Write(payload)
	}
}

func transportHeaderForResponse(req *request, info *transportInfo) string {
	if info.LowerTransport == "tcp" && info.Interleaved != "" {
		return fmt.Sprintf("RTP/AVP/TCP;%s;interleaved=%s", info.Mode, info.Interleaved)
	}
	return fmt.Sprintf("RTP/AVP/UDP;%s;client_port=%d-%d;server_port=5006-5007", info.Mode, info.ClientRTPPort, info.ClientRTCPPort)
}
