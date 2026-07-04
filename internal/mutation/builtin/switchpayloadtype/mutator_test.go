package switchpayloadtype

import (
	"context"
	"testing"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestImmediatePTSwitch(t *testing.T) {
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

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "pt-immediate",
		Kind:    "switch-payloadtype",
		Enabled: true,
		Options: map[string]string{
			"pt.switch.at":    "100",
			"pt.switch.to":    "99",
			"pt.switch.mode":  "immediate",
			"pt.switch.count": "1",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Check metadata indicates PT switch
	if updated.Metadata["mutation.switch-payloadtype"] == "" {
		t.Fatalf("mutation.switch-payloadtype should not be empty")
	}

	// Find the packet at position 100 and check it has PT override
	found := false
	for _, event := range updated.Timeline {
		if event.Metadata != nil {
			if _, ok := event.Metadata["override.pt"]; ok {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected to find PT override in timeline")
	}
}

func TestAlternatePTMode(t *testing.T) {
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

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "pt-alternate",
		Kind:    "switch-payloadtype",
		Enabled: true,
		Options: map[string]string{
			"pt.switch.at":                 "50",
			"pt.switch.to":                 "97",
			"pt.switch.mode":               "alternate",
			"pt.switch.count":              "1",
			"pt.switch.alternate-interval": "20",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Alternate mode should have multiple PT overrides
	overrideCount := 0
	for _, event := range updated.Timeline {
		if event.Metadata != nil {
			if _, ok := event.Metadata["override.pt"]; ok {
				overrideCount++
			}
		}
	}
	if overrideCount < 2 {
		t.Fatalf("alternate mode should have multiple PT overrides, got %d", overrideCount)
	}
}

func TestPTWithDecimalValue(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "pt-decimal",
		Kind:    "switch-payloadtype",
		Enabled: true,
		Options: map[string]string{
			"pt.switch.at":    "50",
			"pt.switch.to":    "100",
			"pt.switch.mode":  "immediate",
			"pt.switch.count": "1",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Check metadata indicates PT switch happened
	if updated.Metadata["mutation.switch-payloadtype"] == "" {
		t.Fatalf("mutation.switch-payloadtype should not be empty")
	}

	if updated.Metadata["pt.history"] == "" {
		t.Fatalf("pt.history should not be empty")
	}
}
