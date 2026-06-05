package droppackets

import (
	"context"
	"testing"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestApplyDropsTimelinePackets(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH265, "pcap", "./sample.pcap", nil)

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "drop-all",
		Kind:    "drop-packets",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(updated.Timeline) != 0 {
		t.Fatalf("timeline length = %d, want 0", len(updated.Timeline))
	}
	if got := updated.Metadata["mutation.drop-packets"]; got != "1" {
		t.Fatalf("metadata mutation.drop-packets = %q, want 1", got)
	}
}
