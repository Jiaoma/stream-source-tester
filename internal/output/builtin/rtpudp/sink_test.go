package rtpudp

import (
	"context"
	"net"
	"testing"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestServeMarksBundleMetadata(t *testing.T) {
	t.Parallel()

	sink := &Sink{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH265, "pcap", "./sample.pcap", nil)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()

	session, err := sink.Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "local-rtp",
		Kind:   "rtp-udp",
		Target: conn.LocalAddr().String(),
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if session == nil {
		t.Fatalf("session should not be nil")
	}
	runtime := session.Result()
	if runtime.SinkKind != "rtp-udp" {
		t.Fatalf("runtime.SinkKind = %q, want rtp-udp", runtime.SinkKind)
	}
	if runtime.State != "ready" && runtime.State != "serving" {
		t.Fatalf("initial runtime.State = %q, want ready or serving", runtime.State)
	}
	if runtime.SessionID == "" {
		t.Fatalf("runtime.SessionID should not be empty")
	}
	if runtime.StartedAt.IsZero() {
		t.Fatalf("runtime.StartedAt should be set")
	}
	host, port, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() error = %v", err)
	}
	if got := runtime.Details["host"]; got != host {
		t.Fatalf("runtime host = %q, want %q", got, host)
	}
	if got := runtime.Details["port"]; got != port {
		t.Fatalf("runtime port = %q, want %q", got, port)
	}
	if got := runtime.Details["payload_type"]; got != "98" {
		t.Fatalf("runtime payload_type = %q, want 98", got)
	}
	if got := runtime.Details["ssrc"]; got != "1" {
		t.Fatalf("runtime ssrc = %q, want 1", got)
	}
	if got := bundle.Metadata["served.by"]; got != "rtp-udp" {
		t.Fatalf("served.by = %q, want rtp-udp", got)
	}
	if got := bundle.Metadata["served.target"]; got != conn.LocalAddr().String() {
		t.Fatalf("served.target = %q, want %q", got, conn.LocalAddr().String())
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for {
		current := session.Result()
		if current.State == "serving" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	buf := make([]byte, 1500)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}
	if n < 13 {
		t.Fatalf("received packet too short: %d", n)
	}
	if version := buf[0] >> 6; version != 2 {
		t.Fatalf("RTP version = %d, want 2", version)
	}
	if payloadType := buf[1] & 0x7f; payloadType != 98 {
		t.Fatalf("payload type = %d, want 98", payloadType)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	closed := session.Result()
	if closed.State != "stopped" {
		t.Fatalf("closed state = %q, want stopped", closed.State)
	}
	if closed.StoppedAt == nil {
		t.Fatalf("closed StoppedAt should be set")
	}
}

func TestServeReplaysTimelineInOrder(t *testing.T) {
	t.Parallel()

	sink := &Sink{}
	bundle := &model.SessionBundle{
		Name:      "timeline",
		Transport: []model.Protocol{model.ProtocolRTPUDP},
		Streams: []model.Stream{{
			ID:          "stream-0",
			Codec:       model.CodecH264,
			ClockRate:   90000,
			PayloadType: 96,
			SSRC:        7,
		}},
		Timeline: []model.PacketEvent{
			{StreamID: "stream-0", Sequence: 10, Timestamp: 1000, Payload: []byte{0xAA}, EmittedAt: 0},
			{StreamID: "stream-0", Sequence: 11, Timestamp: 2000, Payload: []byte{0xBB}, EmittedAt: 30 * time.Millisecond},
		},
		Metadata: map[string]string{},
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()

	session, err := sink.Serve(context.Background(), bundle, config.OutputConfig{
		Name:   "timeline-rtp",
		Kind:   "rtp-udp",
		Target: conn.LocalAddr().String(),
	})
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

	seq1 := uint16(packets[0][2])<<8 | uint16(packets[0][3])
	seq2 := uint16(packets[1][2])<<8 | uint16(packets[1][3])
	if seq1 != 10 || seq2 != 11 {
		t.Fatalf("sequence order = [%d %d], want [10 11]", seq1, seq2)
	}
	ts1 := uint32(packets[0][4])<<24 | uint32(packets[0][5])<<16 | uint32(packets[0][6])<<8 | uint32(packets[0][7])
	ts2 := uint32(packets[1][4])<<24 | uint32(packets[1][5])<<16 | uint32(packets[1][6])<<8 | uint32(packets[1][7])
	if ts1 != 1000 || ts2 != 2000 {
		t.Fatalf("timestamp order = [%d %d], want [1000 2000]", ts1, ts2)
	}
	if packets[0][12] != 0xAA || packets[1][12] != 0xBB {
		t.Fatalf("payload order = [%x %x], want [aa bb]", packets[0][12], packets[1][12])
	}
}
