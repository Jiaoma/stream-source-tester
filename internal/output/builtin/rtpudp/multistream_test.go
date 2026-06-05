package rtpudp

import (
	"context"
	"net"
	"testing"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestServeEmitsPacketsForMultipleStreams(t *testing.T) {
	t.Parallel()

	bundle := model.NewMinimalMultiStreamBundle("multi-rtp", "mp4", "./sample.mp4", nil, []model.MinimalStreamSpec{
		{ID: "video-main", Kind: model.MediaVideo, Codec: model.CodecH264, SSRC: 11, Payload: []byte{0xA1}},
		{ID: "audio-main", Kind: model.MediaAudio, Codec: model.CodecUnknown, SSRC: 22, Payload: []byte{0xB2}},
	})

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "multi-rtp", Kind: "rtp-udp", Target: conn.LocalAddr().String()})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	packets := make([][]byte, 0, 2)
	for len(packets) < 2 {
		buf := make([]byte, 1500)
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			t.Fatalf("ReadFrom() error = %v", err)
		}
		packets = append(packets, append([]byte(nil), buf[:n]...))
	}

	pt1 := packets[0][1] & 0x7f
	pt2 := packets[1][1] & 0x7f
	if pt1 != 96 || pt2 != 98 {
		t.Fatalf("payload types = [%d %d], want [96 98]", pt1, pt2)
	}
	ssrc1 := uint32(packets[0][8])<<24 | uint32(packets[0][9])<<16 | uint32(packets[0][10])<<8 | uint32(packets[0][11])
	ssrc2 := uint32(packets[1][8])<<24 | uint32(packets[1][9])<<16 | uint32(packets[1][10])<<8 | uint32(packets[1][11])
	if ssrc1 != 11 || ssrc2 != 22 {
		t.Fatalf("ssrc values = [%d %d], want [11 22]", ssrc1, ssrc2)
	}
	if packets[0][12] != 0xA1 || packets[1][12] != 0xB2 {
		t.Fatalf("payload bytes = [%x %x], want [a1 b2]", packets[0][12], packets[1][12])
	}
}
