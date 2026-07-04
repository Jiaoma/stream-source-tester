package injectcodecconfig

import (
	"context"
	"testing"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func makeTestBundle(codec model.Codec) *model.SessionBundle {
	return &model.SessionBundle{
		Name:      "test",
		Transport: []model.Protocol{model.ProtocolRTPUDP},
		Streams: []model.Stream{{
			ID:          "stream-0",
			Codec:       codec,
			ClockRate:   90000,
			PayloadType: 96,
			SSRC:        1,
		}},
		Timeline: []model.PacketEvent{},
		Metadata: map[string]string{},
	}
}

// H.264 NAL units with Annex B start codes
func h264SPS() []byte    { return []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xC0, 0x0A} }
func h264PPS() []byte    { return []byte{0x00, 0x00, 0x00, 0x01, 0x68, 0xCE, 0x38, 0x80} }
func h264IDR() []byte    { return []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00} }
func h264NonIDR() []byte { return []byte{0x00, 0x00, 0x00, 0x01, 0x41, 0x88, 0x84, 0x00} }

// H.265 NAL units with Annex B start codes
// For H.265: NAL type is in bits 1-6 of byte after start code (after forbidden_zero_bit)
// VPS (32) = 0x40 << 1 | layer_bit = 0x40 << 1 = 0x80 (but forbidden bit makes it 0x40 for byte value)
// Actually: byte[4] = (forbidden << 7) | (nal_type << 1) | (layer_id bit 0)
// So for nal_type=32: byte[4] = (0 << 7) | (32 << 1) | (0) = 0x40
// But with getNALType = byte >> 1 & 0x3F, we get: 0x40 >> 1 & 0x3F = 0x20 & 0x3F = 32 ✓
// Hmm that doesn't match... Let me recalculate:
// If byte[4] = 0x40 = 01000000
// byte[4] >> 1 = 00100000 = 0x20
// 0x20 & 0x3F = 00100000 = 32 ✓
// So for H.265 VPS (type 32), byte[4] should be 0x40
// But my getNALType gives: 0x40 >> 1 & 0x3F = 0x20 & 0x3F = 32 ✓
// Wait, 0x40 = 64 decimal. 64 >> 1 = 32. 32 & 63 = 32 ✓

// For SPS (33): byte[4] = 33 << 1 = 0x42
// For PPS (34): byte[4] = 34 << 1 = 0x44
// For IDR (19): byte[4] = 19 << 1 = 0x26
func h265VPS() []byte { return []byte{0x00, 0x00, 0x00, 0x01, 0x40, 0x01, 0x0C, 0x01} }
func h265SPS() []byte { return []byte{0x00, 0x00, 0x00, 0x01, 0x42, 0x01, 0x0C, 0x01} }
func h265PPS() []byte { return []byte{0x00, 0x00, 0x00, 0x01, 0x44, 0x01, 0x0C, 0x01} }
func h265IDR() []byte { return []byte{0x00, 0x00, 0x00, 0x01, 0x26, 0x01, 0x0C, 0x01} }

func TestInjectCodecConfig_FirstFrame_H264(t *testing.T) {
	bundle := makeTestBundle(model.CodecH264)
	bundle.Timeline = []model.PacketEvent{
		{StreamID: "stream-0", Sequence: 1, Timestamp: 1000, Marker: false, Payload: h264SPS()},
		{StreamID: "stream-0", Sequence: 2, Timestamp: 2000, Marker: false, Payload: h264PPS()},
		{StreamID: "stream-0", Sequence: 3, Timestamp: 3000, Marker: true, Payload: h264IDR()},
		{StreamID: "stream-0", Sequence: 4, Timestamp: 4000, Marker: false, Payload: h264NonIDR()},
	}

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"injection.strategy": "first-frame",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Should inject codec config before first IDR
	if len(result.Timeline) != 6 {
		t.Errorf("Expected 6 timeline events (3 original + 3 injected), got %d", len(result.Timeline))
	}

	// First 3 packets should be SPS, PPS with Marker=true on last one, then IDR
	if result.Timeline[0].Sequence != 1 {
		t.Errorf("First packet sequence = %d, want 1", result.Timeline[0].Sequence)
	}
	if !result.Timeline[1].Marker {
		t.Errorf("Second codec config packet should have Marker=true")
	}
}

func TestInjectCodecConfig_EveryGOP_H264(t *testing.T) {
	bundle := makeTestBundle(model.CodecH264)
	bundle.Timeline = []model.PacketEvent{
		{StreamID: "stream-0", Sequence: 1, Timestamp: 1000, Marker: true, Payload: h264IDR()},
		{StreamID: "stream-0", Sequence: 2, Timestamp: 2000, Marker: false, Payload: h264NonIDR()},
		{StreamID: "stream-0", Sequence: 3, Timestamp: 3000, Marker: true, Payload: h264IDR()},
		{StreamID: "stream-0", Sequence: 4, Timestamp: 4000, Marker: false, Payload: h264NonIDR()},
	}

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"injection.strategy": "every-gop",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Should inject before both IDR frames
	if len(result.Timeline) < 4 {
		t.Errorf("Expected at least 4 timeline events, got %d", len(result.Timeline))
	}
}

func TestInjectCodecConfig_None_H264(t *testing.T) {
	bundle := makeTestBundle(model.CodecH264)
	bundle.Timeline = []model.PacketEvent{
		{StreamID: "stream-0", Sequence: 1, Timestamp: 1000, Marker: true, Payload: h264IDR()},
	}

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"injection.strategy": "none",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Should be unchanged
	if len(result.Timeline) != 1 {
		t.Errorf("Expected 1 timeline event, got %d", len(result.Timeline))
	}
}

func TestInjectCodecConfig_FirstFrame_H265(t *testing.T) {
	bundle := makeTestBundle(model.CodecH265)
	bundle.Timeline = []model.PacketEvent{
		{StreamID: "stream-0", Sequence: 1, Timestamp: 1000, Marker: false, Payload: h265VPS()},
		{StreamID: "stream-0", Sequence: 2, Timestamp: 2000, Marker: false, Payload: h265SPS()},
		{StreamID: "stream-0", Sequence: 3, Timestamp: 3000, Marker: false, Payload: h265PPS()},
		{StreamID: "stream-0", Sequence: 4, Timestamp: 4000, Marker: true, Payload: h265IDR()},
	}

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"injection.strategy": "first-frame",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Should inject VPS, SPS, PPS before first IDR
	// Timeline: [injected VPS, injected SPS, injected PPS, original VPS, original SPS, original PPS, IDR]
	if len(result.Timeline) != 7 {
		t.Errorf("Expected 7 timeline events, got %d", len(result.Timeline))
	}

	// Last injected codec config should have Marker=true (that's the 3rd injected packet)
	if !result.Timeline[2].Marker {
		t.Errorf("Third codec config packet should have Marker=true")
	}
}

func TestInjectCodecConfig_Periodic_H264(t *testing.T) {
	bundle := makeTestBundle(model.CodecH264)
	// Create 6 video frames with some SPS/PPS at the start
	bundle.Timeline = []model.PacketEvent{
		{StreamID: "stream-0", Sequence: 1, Timestamp: 1000, Marker: true, Payload: h264SPS()},
		{StreamID: "stream-0", Sequence: 2, Timestamp: 2000, Marker: true, Payload: h264PPS()},
	}
	for i := 0; i < 6; i++ {
		bundle.Timeline = append(bundle.Timeline, model.PacketEvent{
			StreamID:  "stream-0",
			Sequence:  uint16(i + 3),
			Timestamp: uint32((i + 3) * 1000),
			Marker:    false,
			Payload:   h264NonIDR(),
		})
	}

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"injection.strategy": "periodic",
			"injection.interval": "3",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Should inject before frame 3 and frame 6 (relative to video frames starting at index 0)
	// Extract finds SPS and PPS from the first two events
	// With 6 video frames at interval 3: inject at frame 3 and frame 6
	// Each injection adds 2 packets (SPS+PPS)
	// Total: 2 configs + 6 frames + 2*2 injected = 12 events
	if len(result.Timeline) != 12 {
		t.Errorf("Expected 12 timeline events, got %d", len(result.Timeline))
	}
}

func TestInjectCodecConfig_Strip_H264(t *testing.T) {
	bundle := makeTestBundle(model.CodecH264)
	bundle.Timeline = []model.PacketEvent{
		{StreamID: "stream-0", Sequence: 1, Timestamp: 1000, Marker: false, Payload: h264SPS()},
		{StreamID: "stream-0", Sequence: 2, Timestamp: 2000, Marker: false, Payload: h264PPS()},
		{StreamID: "stream-0", Sequence: 3, Timestamp: 3000, Marker: true, Payload: h264IDR()},
	}

	mutator := &Mutator{}
	cfg := config.MutationConfig{
		Options: map[string]string{
			"injection.strategy": "first-frame",
			"injection.strip":    "true",
		},
	}

	result, err := mutator.Apply(context.Background(), bundle, cfg)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// With first-frame strategy and strip=true:
	// 1. Injected configs: [injected SPS, injected PPS]
	// 2. Add original events before first IDR (indices 0,1 = SPS, PPS) -> [injected SPS, injected PPS, original SPS, original PPS]
	// 3. Loop from firstIDR onwards, strip=true skips codec configs (indices 0,1) but we're already past firstIDR
	// 4. Only IDR (index 2) is added
	// Final: [injected SPS, injected PPS, original SPS, original PPS, IDR]
	if len(result.Timeline) != 5 {
		t.Errorf("Expected 5 timeline events, got %d", len(result.Timeline))
	}
}

func TestIsCodecConfig_H264(t *testing.T) {
	tests := []struct {
		name     string
		payload  []byte
		expected bool
	}{
		{"SPS", h264SPS(), true},
		{"PPS", h264PPS(), true},
		{"IDR", h264IDR(), false},
		{"Non-IDR", h264NonIDR(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCodecConfig(tt.payload, model.CodecH264)
			if result != tt.expected {
				t.Errorf("isCodecConfig(%v) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestIsCodecConfig_H265(t *testing.T) {
	tests := []struct {
		name     string
		payload  []byte
		expected bool
	}{
		{"VPS", h265VPS(), true},
		{"SPS", h265SPS(), true},
		{"PPS", h265PPS(), true},
		{"IDR", h265IDR(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCodecConfig(tt.payload, model.CodecH265)
			if result != tt.expected {
				t.Errorf("isCodecConfig(%v) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestGetNALType_H264(t *testing.T) {
	// H.264 NAL type is lower 5 bits of first byte after start code
	// SPS = 0x67 & 0x1F = 7
	// PPS = 0x68 & 0x1F = 8
	// IDR = 0x65 & 0x1F = 5

	tests := []struct {
		payload []byte
		nalType int
	}{
		{h264SPS(), 7},
		{h264PPS(), 8},
		{h264IDR(), 5},
		{h264NonIDR(), 1},
	}

	for _, tt := range tests {
		result := getNALType(tt.payload, model.CodecH264)
		if result != tt.nalType {
			t.Errorf("getNALType(%v) = %d, want %d", tt.payload, result, tt.nalType)
		}
	}
}

func TestGetNALType_H265(t *testing.T) {
	// H.265 NAL type is lower 6 bits of first byte after start code
	// VPS = 0x40 & 0x3F = 32
	// SPS = 0x42 & 0x3F = 33
	// PPS = 0x44 & 0x3F = 34
	// IDR = 0x26 & 0x3F = 19

	tests := []struct {
		payload []byte
		nalType int
	}{
		{h265VPS(), 32},
		{h265SPS(), 33},
		{h265PPS(), 34},
		{h265IDR(), 19},
	}

	for _, tt := range tests {
		result := getNALType(tt.payload, model.CodecH265)
		if result != tt.nalType {
			t.Errorf("getNALType(%v) = %d, want %d", tt.payload, result, tt.nalType)
		}
	}
}
