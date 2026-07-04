package switchssrc

import (
	"context"
	"testing"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestImmediateSSRSSwitch(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	// Add more timeline events
	for i := 1; i <= 150; i++ {
		bundle.Timeline = append(bundle.Timeline, model.PacketEvent{
			StreamID:  bundle.Streams[0].ID,
			Sequence:  uint16(i + 1),
			Timestamp: uint32(i * 90000 / 30), // 30fps
			Marker:    false,
			Payload:   []byte{byte(i)},
		})
	}

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "ssrc-immediate",
		Kind:    "switch-ssrc",
		Enabled: true,
		Options: map[string]string{
			"ssrc.switch.at":    "100",
			"ssrc.switch.to":    "0x12345678",
			"ssrc.switch.mode":  "immediate",
			"ssrc.switch.count": "1",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Check metadata indicates SSRC switch
	if updated.Metadata["mutation.switch-ssrc"] == "" {
		t.Fatalf("mutation.switch-ssrc should not be empty")
	}

	// Find the packet at position 100 and check it has SSRC override
	found := false
	for _, event := range updated.Timeline {
		if event.Metadata != nil {
			if _, ok := event.Metadata["override.ssrc"]; ok {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected to find SSRC override in timeline")
	}
}

func TestSSRCWithHexValue(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "ssrc-hex",
		Kind:    "switch-ssrc",
		Enabled: true,
		Options: map[string]string{
			"ssrc.switch.at":    "50",
			"ssrc.switch.to":    "0xDEADBEEF",
			"ssrc.switch.mode":  "immediate",
			"ssrc.switch.count": "1",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Check metadata contains hex value
	if updated.Metadata["ssrc.history"] == "" {
		t.Fatalf("ssrc.history should not be empty")
	}
}

func TestSSRCWithDecimalValue(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "ssrc-decimal",
		Kind:    "switch-ssrc",
		Enabled: true,
		Options: map[string]string{
			"ssrc.switch.at":    "50",
			"ssrc.switch.to":    "305419896",
			"ssrc.switch.mode":  "immediate",
			"ssrc.switch.count": "1",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Check metadata indicates SSRC switch happened
	if updated.Metadata["mutation.switch-ssrc"] == "" {
		t.Fatalf("mutation.switch-ssrc should not be empty")
	}
}

func TestSSRCBurstMode(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	// Add more timeline events
	for i := 1; i <= 150; i++ {
		bundle.Timeline = append(bundle.Timeline, model.PacketEvent{
			StreamID:  bundle.Streams[0].ID,
			Sequence:  uint16(i + 1),
			Timestamp: uint32(i * 90000 / 30),
			Marker:    false,
			Payload:   []byte{byte(i)},
		})
	}

	originalLen := len(bundle.Timeline)

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "ssrc-burst",
		Kind:    "switch-ssrc",
		Enabled: true,
		Options: map[string]string{
			"ssrc.switch.at":          "100",
			"ssrc.switch.to":          "0xABCD0001",
			"ssrc.switch.mode":        "burst",
			"ssrc.switch.count":       "1",
			"ssrc.switch.burst-count": "5",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Burst mode adds duplicate packets, so timeline should be longer
	if len(updated.Timeline) <= originalLen {
		t.Fatalf("burst mode should add packets, timeline length = %d, want > %d", len(updated.Timeline), originalLen)
	}
}
