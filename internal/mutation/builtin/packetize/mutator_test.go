package packetize

import (
	"context"
	"testing"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestFU_AFragmentation(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	// Create a large NALU that needs fragmentation (> MTU of 1400)
	largePayload := make([]byte, 2000)
	// H.264 NAL header (Annex B start code + NAL header)
	largePayload[0] = 0x00
	largePayload[1] = 0x00
	largePayload[2] = 0x00
	largePayload[3] = 0x01
	largePayload[4] = 0x67 // SPS NAL type

	// Fill rest with dummy data
	for i := 5; i < len(largePayload); i++ {
		largePayload[i] = byte(i % 256)
	}

	bundle.Timeline[0].Payload = largePayload

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "fu-a-test",
		Kind:    "packetize",
		Enabled: true,
		Options: map[string]string{"packetize.mode": "fu-a"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// FU-A should produce multiple fragments
	if len(updated.Timeline) <= 1 {
		t.Fatalf("fu-a mode should fragment large NALU, got %d events", len(updated.Timeline))
	}

	// Check metadata indicates FU-A
	if updated.Metadata["mutation.packetize"] != "fu-a" {
		t.Fatalf("mutation.packetize = %q, want fu-a", updated.Metadata["mutation.packetize"])
	}
}

func TestSingleNALUNoFragmentation(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	// Create a small NALU that doesn't need fragmentation
	smallPayload := []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xE0, 0x1E, 0xDB, 0x00}
	bundle.Timeline[0].Payload = smallPayload

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "single-nalu-test",
		Kind:    "packetize",
		Enabled: true,
		Options: map[string]string{"packetize.mode": "single-nalu"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Single NALU should produce one event (no fragmentation)
	if len(updated.Timeline) != 1 {
		t.Fatalf("single-nalu mode should not fragment small NALU, got %d events", len(updated.Timeline))
	}
}

func TestSTAPAAggregation(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	// Create multiple small NALUs that can be aggregated
	smallNalu1 := []byte{0x00, 0x00, 0x00, 0x01, 0x68, 0xCE, 0x38, 0x80} // SPS
	smallNalu2 := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00} // IDR slice

	bundle.Timeline = append(bundle.Timeline, model.PacketEvent{
		StreamID:  bundle.Streams[0].ID,
		Sequence:  2,
		Timestamp: 0,
		Marker:    true,
		Payload:   append(smallNalu1, smallNalu2...),
	})

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "stap-a-test",
		Kind:    "packetize",
		Enabled: true,
		Options: map[string]string{
			"packetize.mode":                "stap-a",
			"packetize.stap.min-aggregated": "2",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// STAP-A should aggregate the NALUs
	if updated.Metadata["mutation.packetize"] != "stap-a" {
		t.Fatalf("mutation.packetize = %q, want stap-a", updated.Metadata["mutation.packetize"])
	}
}

func TestMixedMode(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	// Large NALU (should use FU-A)
	largePayload := make([]byte, 2000)
	largePayload[0] = 0x00
	largePayload[1] = 0x00
	largePayload[2] = 0x00
	largePayload[3] = 0x01
	largePayload[4] = 0x67
	for i := 5; i < len(largePayload); i++ {
		largePayload[i] = byte(i % 256)
	}

	// Small NALU (should use STAP-A)
	smallPayload := []byte{0x00, 0x00, 0x00, 0x01, 0x68, 0xCE, 0x38, 0x80}

	bundle.Timeline[0].Payload = largePayload
	bundle.Timeline = append(bundle.Timeline, model.PacketEvent{
		StreamID:  bundle.Streams[0].ID,
		Sequence:  2,
		Timestamp: 0,
		Marker:    true,
		Payload:   smallPayload,
	})

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "mixed-test",
		Kind:    "packetize",
		Enabled: true,
		Options: map[string]string{
			"packetize.mode":         "mixed",
			"packetize.fu.threshold": "1400",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if updated.Metadata["mutation.packetize"] != "mixed" {
		t.Fatalf("mutation.packetize = %q, want mixed", updated.Metadata["mutation.packetize"])
	}
}

func TestExtractNALUsWithStartCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		payload  []byte
		expected int
	}{
		{
			name:     "Annex B 4-byte start code",
			payload:  []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xE0, 0x1E, 0x00, 0x00, 0x00, 0x01, 0x68, 0xCE, 0x38, 0x80},
			expected: 2,
		},
		{
			name:     "Annex B 3-byte start code",
			payload:  []byte{0x00, 0x00, 0x01, 0x67, 0x42, 0xE0, 0x1E, 0x00, 0x00, 0x01, 0x68, 0xCE, 0x38, 0x80},
			expected: 2,
		},
		{
			name:     "Single NALU without start code",
			payload:  []byte{0x67, 0x42, 0xE0, 0x1E, 0xDB, 0x00},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nalus := extractNALUs(tt.payload, false)
			if len(nalus) != tt.expected {
				t.Errorf("extractNALUs() got %d NALUs, want %d", len(nalus), tt.expected)
			}
		})
	}
}
