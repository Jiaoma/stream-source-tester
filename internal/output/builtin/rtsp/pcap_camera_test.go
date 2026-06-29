package rtsp

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestPcapVirtualCameraWithAuth(t *testing.T) {
	ResetListenerRegistryForTest()

	// Create a minimal pcap-backed bundle (real pcap parsing tested elsewhere)
	bundle := &model.SessionBundle{
		Name:      "pcap-cam",
		Transport: []model.Protocol{model.ProtocolRTSP},
		Streams:   []model.Stream{{ID: "stream-0", Kind: model.MediaVideo, Codec: model.CodecH264, ClockRate: 90000, PayloadType: 96, SSRC: 1}},
		Timeline:  []model.PacketEvent{{StreamID: "stream-0", Sequence: 1, Timestamp: 100, Payload: []byte{0x01, 0x02}, EmittedAt: 0}},
		Metadata: map[string]string{
			"source.kind":     "pcap",
			"source.location": "./fixtures/sample.pcap",
		},
	}

	cfg := config.OutputConfig{
		Name:   "pcap-cam",
		Kind:   "rtsp",
		Target: "rtsp://127.0.0.1:0/pcap-cam",
		Options: map[string]string{
			"auth.mode":     "basic",
			"auth.username": "camera",
			"auth.password": "secret",
			"auth.realm":    "pcap-virtual-camera",
		},
	}

	session, err := (&Sink{}).Serve(context.Background(), bundle, cfg)
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
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	reader := bufio.NewReader(conn)

	// Unauthenticated DESCRIBE should return 401
	resp := sendRTSP(t, conn, reader, "DESCRIBE rtsp://127.0.0.1/pcap-cam RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	if !strings.Contains(resp, "401 Unauthorized") {
		t.Fatalf("unauthenticated DESCRIBE = %q, want 401", resp)
	}

	// Authenticated DESCRIBE should succeed
	// "camera:secret" base64 = Y2FtZXJhOnNlY3JldA==
	resp = sendRTSP(t, conn, reader, "DESCRIBE rtsp://127.0.0.1/pcap-cam RTSP/1.0\r\nCSeq: 2\r\nAuthorization: Basic Y2FtZXJhOnNlY3JldA==\r\n\r\n")
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("authenticated DESCRIBE = %q, want 200 OK", resp)
	}
	if !strings.Contains(resp, "application/sdp") {
		t.Fatalf("DESCRIBE response missing SDP: %q", resp)
	}

	// SET_PARAMETER keepalive should work
	resp = sendRTSP(t, conn, reader, "SET_PARAMETER rtsp://127.0.0.1/pcap-cam RTSP/1.0\r\nCSeq: 3\r\nAuthorization: Basic Y2FtZXJhOnNlY3JldA==\r\n\r\n")
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("SET_PARAMETER keepalive = %q, want 200 OK", resp)
	}
}
