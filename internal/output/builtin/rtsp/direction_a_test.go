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
	"stream-source-tester/internal/output"
)

func TestDescribeIncludesPerStreamFmtp(t *testing.T) {
	t.Parallel()

	bundle := &model.SessionBundle{
		Name:      "fmtp",
		Transport: []model.Protocol{model.ProtocolRTSP},
		Streams: []model.Stream{{
			ID:          "stream-0",
			Kind:        model.MediaVideo,
			Codec:       model.CodecH264,
			ClockRate:   90000,
			PayloadType: 96,
			SSRC:        1,
			Parameters: map[string]string{
				"packetization-mode": "1",
				"profile-level-id":   "42e01f",
			},
		}},
		Metadata: map[string]string{},
	}

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "fmtp", Kind: "rtsp", Target: "rtsp://127.0.0.1:0/fmtp"})
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

	request := "DESCRIBE rtsp://127.0.0.1/fmtp RTSP/1.0\r\nCSeq: 1\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error = %v", err)
	}
	response := string(buf[:n])
	if !strings.Contains(response, "a=fmtp:96 packetization-mode=1;profile-level-id=42e01f") && !strings.Contains(response, "a=fmtp:96 profile-level-id=42e01f;packetization-mode=1") {
		t.Fatalf("response missing fmtp line: %q", response)
	}
}

func TestSetupReturnsServerPorts(t *testing.T) {
	t.Parallel()

	bundle := model.NewMinimalSessionBundle("server-ports", model.CodecH264, "mp4", "./sample.mp4", nil)
	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "server-ports", Kind: "rtsp", Target: "rtsp://127.0.0.1:0/server-ports"})
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

	request := "SETUP rtsp://127.0.0.1/server-ports/streamid=0 RTSP/1.0\r\nCSeq: 1\r\nTransport: RTP/AVP/UDP;unicast;client_port=5004-5005\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error = %v", err)
	}
	response := string(buf[:n])
	if !strings.Contains(response, "server_port=5006-5007") {
		t.Fatalf("response missing server_port: %q", response)
	}
}

func TestPlayCanSendInterleavedRtpOverRtsp(t *testing.T) {
	t.Parallel()

	bundle := &model.SessionBundle{
		Name:      "interleaved",
		Transport: []model.Protocol{model.ProtocolRTSP},
		Streams: []model.Stream{{
			ID:          "stream-0",
			Kind:        model.MediaVideo,
			Codec:       model.CodecH264,
			ClockRate:   90000,
			PayloadType: 96,
			SSRC:        33,
		}},
		Timeline: []model.PacketEvent{{StreamID: "stream-0", Sequence: 5, Timestamp: 1234, Payload: []byte{0xAB}, EmittedAt: 0}},
		Metadata: map[string]string{},
	}

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "interleaved-rtsp", Kind: "rtsp", Target: "rtsp://127.0.0.1:0/interleaved"})
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

	setup := "SETUP rtsp://127.0.0.1/interleaved/streamid=0 RTSP/1.0\r\nCSeq: 1\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n\r\n"
	if _, err := conn.Write([]byte(setup)); err != nil {
		t.Fatalf("Write SETUP error = %v", err)
	}
	buf := make([]byte, 2048)
	if _, err := conn.Read(buf); err != nil && err != io.EOF {
		t.Fatalf("Read SETUP error = %v", err)
	}

	play := "PLAY rtsp://127.0.0.1/interleaved RTSP/1.0\r\nCSeq: 2\r\nSession: rtsp:interleaved\r\n\r\n"
	if _, err := conn.Write([]byte(play)); err != nil {
		t.Fatalf("Write PLAY error = %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read PLAY response error = %v", err)
	}
	payload := buf[:n]
	response := string(payload)
	if !strings.Contains(response, "RTSP/1.0 200 OK") {
		t.Fatalf("PLAY response = %q, want 200 OK", response)
	}
	interleavedFound := false
	for i := 0; i < len(payload); i++ {
		if payload[i] == 0x24 {
			interleavedFound = true
			break
		}
	}
	if !interleavedFound {
		deadline := time.Now().Add(500 * time.Millisecond)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, err = conn.Read(buf)
			if err != nil && err != io.EOF {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					if time.Now().After(deadline) {
						t.Fatalf("Read interleaved packet timeout: %v", err)
					}
					continue
				}
				t.Fatalf("Read interleaved packet error = %v", err)
			}
			payload = buf[:n]
			if len(payload) >= 4 && payload[0] == 0x24 {
				interleavedFound = true
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("did not receive interleaved frame, last payload=%q", string(payload))
			}
		}
	}
	if !interleavedFound {
		t.Fatalf("expected interleaved RTP frame")
	}
}

func waitServingAddr(t *testing.T, session interface{ Result() output.RuntimeResult }) string {
	t.Helper()
	deadline := time.Now().Add(300 * time.Millisecond)
	for {
		current := session.Result()
		if current.State == "serving" {
			return current.Details["listen_address"]
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
