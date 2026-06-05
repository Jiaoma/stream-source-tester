package app

import (
	"path/filepath"
	"testing"

	"stream-source-tester/internal/config"
)

func TestProtocolAnomalyScenariosConfigLoads(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join("..", "..", "examples", "protocol-anomalies.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Name != "protocol-anomalies" {
		t.Fatalf("cfg.Name = %q, want protocol-anomalies", cfg.Name)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("len(cfg.Profiles) = %d, want 2", len(cfg.Profiles))
	}
}
