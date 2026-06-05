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

func TestConfiguredNon200PlaySuppressesRtp(t *testing.T) {
	t.Parallel()

	rtpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer rtpConn.Close()

	bundle := model.NewMinimalSessionBundle("bad-play", model.CodecH264, "mp4", "./sample.mp4", nil)
	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "bad-play",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/test",
		Options: map[string]string{
			"methodStatus.PLAY": "454 Session Not Found",
			"rtpTarget":         rtpConn.LocalAddr().String(),
		},
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	addr := waitServingAddr(t, session)
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("DialTimeout() error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1024)

	setup := "SETUP rtsp://127.0.0.1/test/streamid=0 RTSP/1.0\r\nCSeq: 1\r\nTransport: RTP/AVP/UDP;unicast;client_port=5004-5005\r\n\r\n"
	if _, err := conn.Write([]byte(setup)); err != nil {
		t.Fatalf("Write SETUP error = %v", err)
	}
	if _, err := conn.Read(buf); err != nil && err != io.EOF {
		t.Fatalf("Read SETUP error = %v", err)
	}

	play := "PLAY rtsp://127.0.0.1/test RTSP/1.0\r\nCSeq: 2\r\nSession: rtsp:bad-play\r\n\r\n"
	if _, err := conn.Write([]byte(play)); err != nil {
		t.Fatalf("Write PLAY error = %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read PLAY error = %v", err)
	}
	response := string(buf[:n])
	if !strings.Contains(response, "454 Session Not Found") {
		t.Fatalf("response = %q, want configured 454", response)
	}

	if err := rtpConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, _, err = rtpConn.ReadFrom(make([]byte, 1500))
	if err == nil {
		t.Fatalf("expected no RTP after non-200 PLAY")
	}
}
