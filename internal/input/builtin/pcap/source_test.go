package pcap

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
		Name:     "captured-pcap",
		Kind:     "pcap",
		Codec:    "h265",
		Location: filepath.Join("..", "..", "..", "..", "fixtures", "sample.pcap"),
		Options: map[string]string{
			"preserveTimestamps": "true",
		},
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if bundle.Name != "captured-pcap" {
		t.Fatalf("bundle name = %q, want captured-pcap", bundle.Name)
	}
	if len(bundle.Streams) != 1 {
		t.Fatalf("len(bundle streams) = %d, want 1", len(bundle.Streams))
	}
	if got := bundle.Metadata["source.format"]; got != "capture/pcap" {
		t.Fatalf("source.format = %q, want capture/pcap", got)
	}
	if got := bundle.Metadata["bundle.mode"]; got != "probed-from-capture" {
		t.Fatalf("bundle.mode = %q, want probed-from-capture", got)
	}
	if got := bundle.Metadata["probe.file_size"]; got == "" {
		t.Fatalf("probe.file_size should not be empty")
	}
	if got := bundle.Metadata["probe.magic"]; got == "" {
		t.Fatalf("probe.magic should not be empty")
	}
	if got := bundle.Metadata["probe.endian"]; got != "little" {
		t.Fatalf("probe.endian = %q, want little", got)
	}
	if got := bundle.Metadata["probe.version_major"]; got != "2" {
		t.Fatalf("probe.version_major = %q, want 2", got)
	}
	if got := bundle.Metadata["probe.version_minor"]; got != "4" {
		t.Fatalf("probe.version_minor = %q, want 4", got)
	}
	if got := bundle.Metadata["probe.snaplen"]; got != "65535" {
		t.Fatalf("probe.snaplen = %q, want 65535", got)
	}
	if got := bundle.Metadata["probe.linktype"]; got != "1" {
		t.Fatalf("probe.linktype = %q, want 1", got)
	}
}
