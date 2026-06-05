package setmarker

import (
	"context"
	"testing"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
)

func TestApplySetsMarkerOnTimeline(t *testing.T) {
	t.Parallel()

	mutator := &Mutator{}
	bundle := model.NewMinimalSessionBundle("sample", model.CodecH264, "mp4", "./sample.mp4", nil)

	updated, err := mutator.Apply(context.Background(), bundle, config.MutationConfig{
		Name:    "marker-on",
		Kind:    "set-marker",
		Enabled: true,
		Options: map[string]string{"value": "true"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(updated.Timeline) == 0 {
		t.Fatalf("timeline should not be empty")
	}
	if !updated.Timeline[0].Marker {
		t.Fatalf("marker = false, want true")
	}
	if got := updated.Metadata["mutation.set-marker"]; got != "true" {
		t.Fatalf("metadata mutation.set-marker = %q, want true", got)
	}
}
