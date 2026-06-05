package app

import (
	"context"
	"path/filepath"
	"testing"

	"stream-source-tester/internal/config"
)

func TestAnomalyScenariosConfigLoadsAndBuilds(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join("..", "..", "examples", "anomaly-scenarios.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Name != "anomaly-scenarios" {
		t.Fatalf("cfg.Name = %q, want anomaly-scenarios", cfg.Name)
	}
	for i := range cfg.Inputs {
		if cfg.Inputs[i].Location == "./fixtures/sample.mp4" {
			cfg.Inputs[i].Location = filepath.Join("..", "..", "fixtures", "sample.mp4")
		}
	}
	if len(cfg.Profiles) != 4 {
		t.Fatalf("len(cfg.Profiles) = %d, want 4", len(cfg.Profiles))
	}

	plan, err := BuildPlan(cfg)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Profiles) != 4 {
		t.Fatalf("len(plan.Profiles) = %d, want 4", len(plan.Profiles))
	}

	bundles, err := InstantiateProfiles(context.Background(), cfg, plan)
	if err != nil {
		t.Fatalf("InstantiateProfiles() error = %v", err)
	}
	if len(bundles) != 4 {
		t.Fatalf("len(bundles) = %d, want 4", len(bundles))
	}

	if _, ok := bundles["normal-udp"]; !ok {
		t.Fatalf("normal-udp bundle missing")
	}
	if _, ok := bundles["marker-and-seq"]; !ok {
		t.Fatalf("marker-and-seq bundle missing")
	}
	if _, ok := bundles["timestamp-and-drop"]; !ok {
		t.Fatalf("timestamp-and-drop bundle missing")
	}
	if _, ok := bundles["reorder-over-rtsp"]; !ok {
		t.Fatalf("reorder-over-rtsp bundle missing")
	}
}
