package mp4

import (
	"context"
	"path/filepath"
	"testing"

	"stream-source-tester/internal/config"
)

func TestOpenReturnsProbedBundle(t *testing.T) {
	t.Parallel()

	source := &Source{}
	bundle, err := source.Open(context.Background(), config.InputConfig{
		Name:     "sample-mp4",
		Kind:     "mp4",
		Codec:    "h264",
		Location: filepath.Join("..", "..", "..", "..", "fixtures", "sample.mp4"),
		Options: map[string]string{
			"loop": "true",
		},
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if bundle.Name != "sample-mp4" {
		t.Fatalf("bundle name = %q, want sample-mp4", bundle.Name)
	}
	if len(bundle.Streams) != 1 {
		t.Fatalf("len(bundle streams) = %d, want 1", len(bundle.Streams))
	}
	if got := bundle.Metadata["source.format"]; got != "container/mp4" {
		t.Fatalf("source.format = %q, want container/mp4", got)
	}
	if got := bundle.Metadata["bundle.mode"]; got != "probed-from-mp4" {
		t.Fatalf("bundle.mode = %q, want probed-from-mp4", got)
	}
	if got := bundle.Metadata["probe.file_size"]; got == "" {
		t.Fatalf("probe.file_size should not be empty")
	}
	if got := bundle.Metadata["probe.header_hex"]; got == "" {
		t.Fatalf("probe.header_hex should not be empty")
	}
	if got := bundle.Metadata["probe.major_brand"]; got != "isom" {
		t.Fatalf("probe.major_brand = %q, want isom", got)
	}
	if got := bundle.Metadata["probe.minor_version"]; got != "512" {
		t.Fatalf("probe.minor_version = %q, want 512", got)
	}
	if got := bundle.Metadata["probe.compatible_brands"]; got != "isom,iso2" {
		t.Fatalf("probe.compatible_brands = %q, want isom,iso2", got)
	}
}
