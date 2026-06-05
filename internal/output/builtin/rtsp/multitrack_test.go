package rtsp

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestDescribeReturnsMultiTrackSDP(t *testing.T) {
	t.Parallel()

	bundle := model.NewMinimalMultiStreamBundle("multi", "mp4", "./sample.mp4", nil, []model.MinimalStreamSpec{
		{ID: "video-main", Kind: model.MediaVideo, Codec: model.CodecH264, SSRC: 11, Payload: []byte{0xA1}},
		{ID: "audio-main", Kind: model.MediaAudio, Codec: model.CodecUnknown, SSRC: 22, Payload: []byte{0xB2}},
	})

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "multi-rtsp", Kind: "rtsp", Target: "rtsp://127.0.0.1:0/multi"})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(300 * time.Millisecond)
	addr := ""
	for {
		current := session.Result()
		if current.State == "serving" {
			addr = current.Details["listen_address"]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	request := "DESCRIBE rtsp://127.0.0.1/multi RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error = %v", err)
	}
	response := string(buf[:n])
	if strings.Count(response, "m=") != 2 {
		t.Fatalf("expected 2 media sections, response=%q", response)
	}
	if !strings.Contains(response, "a=control:streamid=0") || !strings.Contains(response, "a=control:streamid=1") {
		t.Fatalf("response missing per-track control lines: %q", response)
	}
	if !strings.Contains(response, "m=video 0 RTP/AVP 96") || !strings.Contains(response, "m=audio 0 RTP/AVP 98") {
		t.Fatalf("response missing media type sections: %q", response)
	}
	if !strings.Contains(response, "a=rtpmap:96 H264/90000") || !strings.Contains(response, "a=rtpmap:98 L16/48000") {
		t.Fatalf("response missing codec mappings: %q", response)
	}
}
