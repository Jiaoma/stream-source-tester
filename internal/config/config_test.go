package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.yaml")
	content := []byte(`name: demo
inputs:
  - name: sample
    kind: mp4
    codec: h264
    location: ./sample.mp4
outputs:
  - name: local
    kind: rtsp
    target: rtsp://127.0.0.1:8554/test
mutations:
  - name: passthrough
    kind: identity
    enabled: true
profiles:
  - name: profile-a
    input: sample
    output: local
    mutations: [passthrough]
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.BindAddress != "0.0.0.0" {
		t.Fatalf("BindAddress = %q, want %q", cfg.Server.BindAddress, "0.0.0.0")
	}
	if cfg.Server.RTSPPort != 8554 {
		t.Fatalf("RTSPPort = %d, want 8554", cfg.Server.RTSPPort)
	}
	if cfg.Server.RTPPortMin != 10000 {
		t.Fatalf("RTPPortMin = %d, want 10000", cfg.Server.RTPPortMin)
	}
	if cfg.Server.RTPPortMax != 10100 {
		t.Fatalf("RTPPortMax = %d, want 10100", cfg.Server.RTPPortMax)
	}
	if cfg.Inputs[0].Options == nil {
		t.Fatalf("input options should be initialized")
	}
	if cfg.Outputs[0].Options == nil {
		t.Fatalf("output options should be initialized")
	}
	if cfg.Mutations[0].Options == nil {
		t.Fatalf("mutation options should be initialized")
	}
}
