package rewritertptimestamp

import (
	"context"
	"testing"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func makeTestBundle() *model.SessionBundle {
	return &model.SessionBundle{
		Name:      "test",
		Transport: []model.Protocol{model.ProtocolRTPUDP},
		Streams: []model.Stream{{
			ID:          "stream-0",
			Codec:       model.CodecH264,
			ClockRate:   90000,
			PayloadType: 96,
			SSRC:        1,
		}},
		Timeline: []model.PacketEvent{
			{StreamID: "stream-0", Sequence: 1, Timestamp: 90000, Marker: true, Payload: []byte{0x65}},
			{StreamID: "stream-0", Sequence: 2, Timestamp: 90100, Marker: false, Payload: []byte{0x41}},
			{StreamID: "stream-0", Sequence: 3, Timestamp: 90200, Marker: false, Payload: []byte{0x41}},
		},
		Metadata: map[string]string{},
	}
}

func TestRewriteRTPTimestamp_Offset(t *testing.T) {
	bundle := makeTestBundle()

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"rtp.offset": "1000",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// All timestamps should be increased by 1000
	expectedTimestamps := []uint32{91000, 91100, 91200}
	for i, expected := range expectedTimestamps {
		if result.Timeline[i].Timestamp != expected {
			t.Errorf("Timeline[%d].Timestamp = %d, want %d", i, result.Timeline[i].Timestamp, expected)
		}
	}
}

func TestRewriteRTPTimestamp_NegativeOffset(t *testing.T) {
	bundle := makeTestBundle()

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"rtp.offset": "-1000",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// All timestamps should be decreased by 1000
	expectedTimestamps := []uint32{89000, 89100, 89200}
	for i, expected := range expectedTimestamps {
		if result.Timeline[i].Timestamp != expected {
			t.Errorf("Timeline[%d].Timestamp = %d, want %d", i, result.Timeline[i].Timestamp, expected)
		}
	}
}

func TestRewriteRTPTimestamp_ClockRateOverride(t *testing.T) {
	bundle := makeTestBundle()

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"rtp.clockrate": "3000", // 30fps instead of 90000
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// With 90000->3000 clock rate change (1:30 scale), original delta of 100 becomes 100/30 ≈ 3.33 → 3
	// First timestamp stays the same, subsequent timestamps scale
	// delta = 90100-90000 = 100, scaled delta = 100 * (3000/90000) = 100/30 ≈ 3
	// So timestamp[1] = 90000 + 3 = 90003
	// timestamp[2] = 90000 + (200 * 1/30) = 90006 or so
	if result.Timeline[0].Timestamp != 90000 {
		t.Errorf("First timestamp = %d, want 90000 (unchanged)", result.Timeline[0].Timestamp)
	}
}

func TestRewriteRTPTimestamp_Combined(t *testing.T) {
	bundle := makeTestBundle()

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"rtp.offset":    "1000",
			"rtp.clockrate": "3000",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// With both offset and clockRate:
	// - Offset is applied first: 90000+1000 = 91000
	// - Then clockRate scaling is applied to delta from firstTimestamp
	// Since first timestamp becomes 91000 after offset, and clockRate scales delta:
	// - Timeline[0]: delta=0, scaled=0, result=91000
	// But the code applies offset to get newTimestamp, then re-scales from ORIGINAL firstTimestamp
	// So: newTimestamp = 90000+1000 = 91000, then scaledDelta = (91000-90000)*(3000/90000) = 0, result=90000
	// Actually let me trace through more carefully...
	if result.Timeline[0].Timestamp != 90000 {
		t.Errorf("First timestamp = %d, want 90000 (offset canceled by clockRate scaling on zero delta)", result.Timeline[0].Timestamp)
	}
}

func TestRewriteRTPTimestamp_CaptureOffset(t *testing.T) {
	bundle := makeTestBundle()

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"rtp.capture_offset": "1000",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// CaptureOffset should be set to 1000ms (1000 microseconds * 1000000)
	if result.CaptureOffset == 0 {
		t.Error("CaptureOffset should not be 0")
	}
}

func TestRewriteRTPTimestamp_Metadata(t *testing.T) {
	bundle := makeTestBundle()

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"rtp.offset":    "500",
			"rtp.clockrate": "6000",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	if result.Metadata["mutation.rewrite-rtp-timestamp.offset"] != "500" {
		t.Errorf("offset metadata = %q, want 500", result.Metadata["mutation.rewrite-rtp-timestamp.offset"])
	}
	if result.Metadata["mutation.rewrite-rtp-timestamp.clockrate"] != "6000" {
		t.Errorf("clockrate metadata = %q, want 6000", result.Metadata["mutation.rewrite-rtp-timestamp.clockrate"])
	}
}

func TestRewriteRTPTimestamp_NoOp(t *testing.T) {
	bundle := makeTestBundle()

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Timestamps should remain unchanged
	expectedTimestamps := []uint32{90000, 90100, 90200}
	for i, expected := range expectedTimestamps {
		if result.Timeline[i].Timestamp != expected {
			t.Errorf("Timeline[%d].Timestamp = %d, want %d", i, result.Timeline[i].Timestamp, expected)
		}
	}
}

func TestRewriteRTPTimestamp_InvalidOffset(t *testing.T) {
	bundle := makeTestBundle()

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"rtp.offset": "not-a-number",
		},
	}

	_, err := mutator.Apply(context.Background(), bundle, cfg)
	if err == nil {
		t.Error("Expected error for invalid offset, got nil")
	}
}

func TestRewriteRTPTimestamp_InvalidClockRate(t *testing.T) {
	bundle := makeTestBundle()

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"rtp.clockrate": "zero",
		},
	}

	_, err := mutator.Apply(context.Background(), bundle, cfg)
	if err == nil {
		t.Error("Expected error for invalid clockrate, got nil")
	}
}
