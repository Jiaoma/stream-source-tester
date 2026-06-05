package rtpudp

import (
	"context"
	"net"
	"testing"
	"time"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
	_ "stream-source-tester/internal/mutation/builtin/dropeverynth"
	_ "stream-source-tester/internal/mutation/builtin/droppackets"
	_ "stream-source-tester/internal/mutation/builtin/reorderfirsttwo"
	_ "stream-source-tester/internal/mutation/builtin/rewritesequence"
	_ "stream-source-tester/internal/mutation/builtin/rewritetimestamp"
	_ "stream-source-tester/internal/mutation/builtin/setmarker"
)

func TestNetworkMutationSetMarkerAffectsPacket(t *testing.T) {
	t.Parallel()

	bundle := &model.SessionBundle{
		Name:      "marker",
		Transport: []model.Protocol{model.ProtocolRTPUDP},
		Streams:   []model.Stream{{ID: "stream-0", Codec: model.CodecH264, ClockRate: 90000, PayloadType: 96, SSRC: 9}},
		Timeline:  []model.PacketEvent{{StreamID: "stream-0", Sequence: 1, Timestamp: 100, Payload: []byte{0xAB}, Marker: false, EmittedAt: 0}},
		Metadata:  map[string]string{},
	}

	mutator, err := mutation.New("set-marker")
	if err != nil {
		t.Fatalf("mutation.New() error = %v", err)
	}
	bundle, err = mutator.Apply(context.Background(), bundle, config.MutationConfig{Kind: "set-marker", Options: map[string]string{"value": "true"}})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "marker-out", Kind: "rtp-udp", Target: conn.LocalAddr().String()})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	buf := make([]byte, 1500)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}
	if n < 12 {
		t.Fatalf("packet too short: %d", n)
	}
	if marker := buf[1] >> 7; marker != 1 {
		t.Fatalf("marker bit = %d, want 1", marker)
	}
}

func TestNetworkMutationRewriteSequenceAffectsPacket(t *testing.T) {
	t.Parallel()

	bundle := &model.SessionBundle{
		Name:      "sequence",
		Transport: []model.Protocol{model.ProtocolRTPUDP},
		Streams:   []model.Stream{{ID: "stream-0", Codec: model.CodecH264, ClockRate: 90000, PayloadType: 96, SSRC: 9}},
		Timeline:  []model.PacketEvent{{StreamID: "stream-0", Sequence: 5, Timestamp: 100, Payload: []byte{0xCD}, EmittedAt: 0}},
		Metadata:  map[string]string{},
	}

	mutator, err := mutation.New("rewrite-sequence")
	if err != nil {
		t.Fatalf("mutation.New() error = %v", err)
	}
	bundle, err = mutator.Apply(context.Background(), bundle, config.MutationConfig{Kind: "rewrite-sequence", Options: map[string]string{"offset": "7"}})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "sequence-out", Kind: "rtp-udp", Target: conn.LocalAddr().String()})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	buf := make([]byte, 1500)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}
	if n < 12 {
		t.Fatalf("packet too short: %d", n)
	}
	seq := uint16(buf[2])<<8 | uint16(buf[3])
	if seq != 12 {
		t.Fatalf("sequence = %d, want 12", seq)
	}
}

func TestNetworkMutationDropPacketsSuppressesOutput(t *testing.T) {
	t.Parallel()

	bundle := &model.SessionBundle{
		Name:      "drop",
		Transport: []model.Protocol{model.ProtocolRTPUDP},
		Streams:   []model.Stream{{ID: "stream-0", Codec: model.CodecH264, ClockRate: 90000, PayloadType: 96, SSRC: 9}},
		Timeline:  []model.PacketEvent{{StreamID: "stream-0", Sequence: 1, Timestamp: 100, Payload: []byte{0xEF}, EmittedAt: 0}},
		Metadata:  map[string]string{},
	}

	mutator, err := mutation.New("drop-packets")
	if err != nil {
		t.Fatalf("mutation.New() error = %v", err)
	}
	bundle, err = mutator.Apply(context.Background(), bundle, config.MutationConfig{Kind: "drop-packets"})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "drop-out", Kind: "rtp-udp", Target: conn.LocalAddr().String()})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	buf := make([]byte, 1500)
	_, _, err = conn.ReadFrom(buf)
	if err == nil {
		t.Fatalf("expected no packets after drop-packets mutation")
	}
}

func TestNetworkMutationRewriteTimestampAffectsPacket(t *testing.T) {
	t.Parallel()

	bundle := &model.SessionBundle{
		Name:      "timestamp",
		Transport: []model.Protocol{model.ProtocolRTPUDP},
		Streams:   []model.Stream{{ID: "stream-0", Codec: model.CodecH264, ClockRate: 90000, PayloadType: 96, SSRC: 9}},
		Timeline:  []model.PacketEvent{{StreamID: "stream-0", Sequence: 1, Timestamp: 100, Payload: []byte{0xA1}, EmittedAt: 0}},
		Metadata:  map[string]string{},
	}

	mutator, err := mutation.New("rewrite-timestamp")
	if err != nil {
		t.Fatalf("mutation.New() error = %v", err)
	}
	bundle, err = mutator.Apply(context.Background(), bundle, config.MutationConfig{Kind: "rewrite-timestamp", Options: map[string]string{"offset": "900"}})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "timestamp-out", Kind: "rtp-udp", Target: conn.LocalAddr().String()})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	defer session.Close()

	if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	buf := make([]byte, 1500)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}
	if n < 12 {
		t.Fatalf("packet too short: %d", n)
	}
	ts := uint32(buf[4])<<24 | uint32(buf[5])<<16 | uint32(buf[6])<<8 | uint32(buf[7])
	if ts != 1000 {
		t.Fatalf("timestamp = %d, want 1000", ts)
	}
}

func TestNetworkMutationDropEveryNthAffectsOutput(t *testing.T) {
	t.Parallel()

	bundle := &model.SessionBundle{
		Name:      "drop-nth",
		Transport: []model.Protocol{model.ProtocolRTPUDP},
		Streams:   []model.Stream{{ID: "stream-0", Codec: model.CodecH264, ClockRate: 90000, PayloadType: 96, SSRC: 9}},
		Timeline: []model.PacketEvent{
			{StreamID: "stream-0", Sequence: 1, Timestamp: 100, Payload: []byte{0xA1}, EmittedAt: 0},
			{StreamID: "stream-0", Sequence: 2, Timestamp: 200, Payload: []byte{0xA2}, EmittedAt: 20 * time.Millisecond},
			{StreamID: "stream-0", Sequence: 3, Timestamp: 300, Payload: []byte{0xA3}, EmittedAt: 40 * time.Millisecond},
		},
		Metadata: map[string]string{},
	}

	mutator, err := mutation.New("drop-every-nth")
	if err != nil {
		t.Fatalf("mutation.New() error = %v", err)
	}
	bundle, err = mutator.Apply(context.Background(), bundle, config.MutationConfig{Kind: "drop-every-nth", Options: map[string]string{"n": "2"}})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "drop-nth-out", Kind: "rtp-udp", Target: conn.LocalAddr().String()})
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
	if seq1 != 1 || seq2 != 3 {
		t.Fatalf("sequence order = [%d %d], want [1 3]", seq1, seq2)
	}
}

func TestNetworkMutationReorderFirstTwoAffectsOutput(t *testing.T) {
	t.Parallel()

	bundle := &model.SessionBundle{
		Name:      "reorder",
		Transport: []model.Protocol{model.ProtocolRTPUDP},
		Streams:   []model.Stream{{ID: "stream-0", Codec: model.CodecH264, ClockRate: 90000, PayloadType: 96, SSRC: 9}},
		Timeline: []model.PacketEvent{
			{StreamID: "stream-0", Sequence: 10, Timestamp: 1000, Payload: []byte{0xB1}, EmittedAt: 0},
			{StreamID: "stream-0", Sequence: 11, Timestamp: 2000, Payload: []byte{0xB2}, EmittedAt: 20 * time.Millisecond},
		},
		Metadata: map[string]string{},
	}

	mutator, err := mutation.New("reorder-first-two")
	if err != nil {
		t.Fatalf("mutation.New() error = %v", err)
	}
	bundle, err = mutator.Apply(context.Background(), bundle, config.MutationConfig{Kind: "reorder-first-two"})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()

	session, err := (&Sink{}).Serve(context.Background(), bundle, config.OutputConfig{Name: "reorder-out", Kind: "rtp-udp", Target: conn.LocalAddr().String()})
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
	if seq1 != 11 || seq2 != 10 {
		t.Fatalf("sequence order = [%d %d], want [11 10]", seq1, seq2)
	}
}
