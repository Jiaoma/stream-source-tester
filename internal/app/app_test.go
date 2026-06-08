package app

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"stream-source-tester/internal/config"
	rtsptest "stream-source-tester/internal/output/builtin/rtsp"
)

func TestRunBlocksUntilContextCancelled(t *testing.T) {
	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "sample.mp4"))
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "quickstart.yaml")
	configContent := []byte("name: quickstart-rtsp\ninputs:\n  - name: local-input\n    kind: mp4\n    codec: h264\n    location: " + fixturePath + "\noutputs:\n  - name: rtsp-out\n    kind: rtsp\n    target: rtsp://127.0.0.1:0/test\nmutations:\n  - name: passthrough\n    kind: identity\n    enabled: true\nprofiles:\n  - name: normal-rtsp\n    input: local-input\n    output: rtsp-out\n    mutations: [passthrough]\n")
	if err := os.WriteFile(configPath, configContent, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, &out, configPath)
	}()

	select {
	case err := <-done:
		t.Fatalf("Run() returned too early: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() after cancel error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run() did not exit after context cancellation")
	}
}

func TestRunListensOnConfiguredRTSPPort(t *testing.T) {
	rtsptest.ResetListenerRegistryForTest()

	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "sample.mp4"))
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	portProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	_, port, err := net.SplitHostPort(portProbe.Addr().String())
	if err != nil {
		_ = portProbe.Close()
		t.Fatalf("SplitHostPort() error = %v", err)
	}
	_ = portProbe.Close()

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "listen.yaml")
	configContent := []byte("name: listen-check\ninputs:\n  - name: local-input\n    kind: mp4\n    codec: h264\n    location: " + fixturePath + "\noutputs:\n  - name: rtsp-out\n    kind: rtsp\n    target: rtsp://127.0.0.1:" + port + "/test\nmutations:\n  - name: passthrough\n    kind: identity\n    enabled: true\nprofiles:\n  - name: normal-rtsp\n    input: local-input\n    output: rtsp-out\n    mutations: [passthrough]\n")
	if err := os.WriteFile(configPath, configContent, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, &out, configPath)
	}()

	listenTarget := "127.0.0.1:" + port
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", listenTarget, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("RTSP port 8554 was not listening before deadline: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() after cancel error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run() did not exit after cancellation")
	}
}

func TestBuildPlanResolvesProfiles(t *testing.T) {
	t.Parallel()

	cfg := testConfig()

	plan, err := BuildPlan(cfg)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if plan.ScenarioName != "demo" {
		t.Fatalf("ScenarioName = %q, want demo", plan.ScenarioName)
	}
	if len(plan.Profiles) != 1 {
		t.Fatalf("len(Profiles) = %d, want 1", len(plan.Profiles))
	}

	profile := plan.Profiles[0]
	if profile.Input.Name != "sample-mp4" {
		t.Fatalf("profile input name = %q, want sample-mp4", profile.Input.Name)
	}
	if profile.Input.Kind != "mp4" {
		t.Fatalf("profile input kind = %q, want mp4", profile.Input.Kind)
	}
	if profile.Input.Codec != "h264" {
		t.Fatalf("profile input codec = %q, want h264", profile.Input.Codec)
	}
	if profile.Output.Name != "local-rtsp" {
		t.Fatalf("profile output name = %q, want local-rtsp", profile.Output.Name)
	}
	if len(profile.Mutations) != 2 {
		t.Fatalf("len(profile mutations) = %d, want 2", len(profile.Mutations))
	}
	if profile.Mutations[0].Name != "marker-on" {
		t.Fatalf("first mutation name = %q, want marker-on", profile.Mutations[0].Name)
	}
	if profile.Mutations[1].Name != "drop-all" {
		t.Fatalf("second mutation name = %q, want drop-all", profile.Mutations[1].Name)
	}
}

func TestInstantiateProfilesAppliesMutations(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	plan, err := BuildPlan(cfg)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	bundles, err := InstantiateProfiles(context.Background(), cfg, plan)
	if err != nil {
		t.Fatalf("InstantiateProfiles() error = %v", err)
	}

	bundle, ok := bundles["profile-a"]
	if !ok {
		t.Fatalf("bundle for profile-a not found")
	}
	if bundle.Name != "profile-a" {
		t.Fatalf("bundle name = %q, want profile-a", bundle.Name)
	}
	if len(bundle.Streams) != 1 {
		t.Fatalf("len(bundle streams) = %d, want 1", len(bundle.Streams))
	}
	if len(bundle.Timeline) != 0 {
		t.Fatalf("timeline should be empty after drop-packets, got %d", len(bundle.Timeline))
	}
	if got := bundle.Metadata["profile.name"]; got != "profile-a" {
		t.Fatalf("profile.name metadata = %q, want profile-a", got)
	}
	if got := bundle.Metadata["output.kind"]; got != "rtsp" {
		t.Fatalf("output.kind metadata = %q, want rtsp", got)
	}
	if got := bundle.Metadata["mutation.set-marker"]; got != "true" {
		t.Fatalf("mutation.set-marker metadata = %q, want true", got)
	}
	if got := bundle.Metadata["mutation.drop-packets"]; got != "1" {
		t.Fatalf("mutation.drop-packets metadata = %q, want 1", got)
	}
}

func TestServeProfilesMarksServedMetadata(t *testing.T) {
	t.Parallel()

	portProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	_, port, err := net.SplitHostPort(portProbe.Addr().String())
	if err != nil {
		_ = portProbe.Close()
		t.Fatalf("SplitHostPort() error = %v", err)
	}
	_ = portProbe.Close()

	cfg := testConfigWithMutationsOnTarget("rtsp://127.0.0.1:" + port + "/test", []config.MutationConfig{
		{Name: "marker-on", Kind: "set-marker", Enabled: true, Options: map[string]string{"value": "true"}},
		{Name: "drop-all", Kind: "drop-packets", Enabled: true},
	})
	plan, err := BuildPlan(cfg)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	bundles, err := InstantiateProfiles(context.Background(), cfg, plan)
	if err != nil {
		t.Fatalf("InstantiateProfiles() error = %v", err)
	}

	manager, err := ServeProfiles(context.Background(), cfg, plan, bundles)
	if err != nil {
		t.Fatalf("ServeProfiles() error = %v", err)
	}

	session, ok := manager.Get("profile-a")
	if !ok {
		t.Fatalf("session for profile-a not found")
	}
	runtime := session.Result()
	if runtime.SinkKind != "rtsp" {
		t.Fatalf("runtime.SinkKind = %q, want rtsp", runtime.SinkKind)
	}
	expectedTarget := "rtsp://127.0.0.1:" + port + "/test"
	if runtime.Target != expectedTarget {
		t.Fatalf("runtime.Target = %q, want %q", runtime.Target, expectedTarget)
	}
	if runtime.State != "ready" && runtime.State != "serving" {
		t.Fatalf("initial runtime.State = %q, want ready or serving", runtime.State)
	}
	if runtime.Timeline != 0 {
		t.Fatalf("runtime.Timeline = %d, want 0", runtime.Timeline)
	}
	if runtime.SessionID == "" {
		t.Fatalf("runtime.SessionID should not be empty")
	}
	if runtime.StartedAt.IsZero() {
		t.Fatalf("runtime.StartedAt should be set")
	}
	if got := runtime.Details["listen_address"]; got == "" {
		t.Fatalf("runtime listen_address should not be empty")
	}
	if got := runtime.Details["mount_path"]; got != "test" {
		t.Fatalf("runtime mount_path = %q, want test", got)
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for {
		current := session.Result()
		if current.State == "serving" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime.State did not transition to serving, last=%q", current.State)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("session.Close() error = %v", err)
	}
	closed := session.Result()
	if closed.State != "stopped" {
		t.Fatalf("closed state = %q, want stopped", closed.State)
	}
	if closed.StoppedAt == nil {
		t.Fatalf("closed StoppedAt should be set")
	}

	bundle := bundles["profile-a"]
	if got := bundle.Metadata["served.by"]; got != "rtsp" {
		t.Fatalf("served.by metadata = %q, want rtsp", got)
	}
	if got := bundle.Metadata["served.target"]; got != expectedTarget {
		t.Fatalf("served.target metadata = %q, want %q", got, expectedTarget)
	}
	if got := bundle.Metadata["served.timeline"]; got != "0" {
		t.Fatalf("served.timeline metadata = %q, want 0", got)
	}
}

func TestBuildPlanResolvesMultiMutationProfile(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithMutations([]config.MutationConfig{
		{Name: "marker-on", Kind: "set-marker", Enabled: true, Options: map[string]string{"value": "true"}},
		{Name: "seq-plus-7", Kind: "rewrite-sequence", Enabled: true, Options: map[string]string{"offset": "7"}},
		{Name: "ts-plus-900", Kind: "rewrite-timestamp", Enabled: true, Options: map[string]string{"offset": "900"}},
	})
	cfg.Profiles = []config.ProfileConfig{{Name: "combo", Input: "sample-mp4", Output: "local-rtsp", Mutations: []string{"marker-on", "seq-plus-7", "ts-plus-900"}}}

	plan, err := BuildPlan(cfg)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Profiles) != 1 {
		t.Fatalf("len(Profiles) = %d, want 1", len(plan.Profiles))
	}
	if len(plan.Profiles[0].Mutations) != 3 {
		t.Fatalf("len(profile mutations) = %d, want 3", len(plan.Profiles[0].Mutations))
	}
	if plan.Profiles[0].Mutations[0].Name != "marker-on" || plan.Profiles[0].Mutations[1].Name != "seq-plus-7" || plan.Profiles[0].Mutations[2].Name != "ts-plus-900" {
		t.Fatalf("unexpected mutation order: %+v", plan.Profiles[0].Mutations)
	}
}

func TestInstantiateProfilesAppliesMutationCombination(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithMutations([]config.MutationConfig{
		{Name: "marker-on", Kind: "set-marker", Enabled: true, Options: map[string]string{"value": "true"}},
		{Name: "seq-plus-7", Kind: "rewrite-sequence", Enabled: true, Options: map[string]string{"offset": "7"}},
		{Name: "ts-plus-900", Kind: "rewrite-timestamp", Enabled: true, Options: map[string]string{"offset": "900"}},
	})
	cfg.Profiles = []config.ProfileConfig{{Name: "combo", Input: "sample-mp4", Output: "local-rtsp", Mutations: []string{"marker-on", "seq-plus-7", "ts-plus-900"}}}

	plan, err := BuildPlan(cfg)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	bundles, err := InstantiateProfiles(context.Background(), cfg, plan)
	if err != nil {
		t.Fatalf("InstantiateProfiles() error = %v", err)
	}
	bundle := bundles["combo"]
	if len(bundle.Timeline) != 1 {
		t.Fatalf("len(bundle timeline) = %d, want 1", len(bundle.Timeline))
	}
	if !bundle.Timeline[0].Marker {
		t.Fatalf("marker should be true after set-marker")
	}
	if bundle.Timeline[0].Sequence != 8 {
		t.Fatalf("sequence = %d, want 8", bundle.Timeline[0].Sequence)
	}
	if bundle.Timeline[0].Timestamp != 900 {
		t.Fatalf("timestamp = %d, want 900", bundle.Timeline[0].Timestamp)
	}
}

func testConfig() *config.Config {
	return testConfigWithMutationsOnTarget("rtsp://127.0.0.1:8554/test", []config.MutationConfig{
		{Name: "marker-on", Kind: "set-marker", Enabled: true, Options: map[string]string{"value": "true"}},
		{Name: "drop-all", Kind: "drop-packets", Enabled: true},
	})
}

func testConfigWithMutations(mutations []config.MutationConfig) *config.Config {
	return testConfigWithMutationsOnTarget("rtsp://127.0.0.1:8554/test", mutations)
}

func testConfigWithMutationsOnTarget(target string, mutations []config.MutationConfig) *config.Config {
	cfg := &config.Config{
		Name: "demo",
		Inputs: []config.InputConfig{
			{
				Name:     "sample-mp4",
				Kind:     "mp4",
				Codec:    "h264",
				Location: "../../fixtures/sample.mp4",
				Options: map[string]string{
					"loop": "true",
				},
			},
		},
		Outputs: []config.OutputConfig{
			{
				Name:   "local-rtsp",
				Kind:   "rtsp",
				Target: target,
			},
		},
		Mutations: mutations,
		Profiles: []config.ProfileConfig{
			{
				Name:      "profile-a",
				Input:     "sample-mp4",
				Output:    "local-rtsp",
				Mutations: []string{"marker-on", "drop-all"},
			},
		},
	}
	cfg.ApplyDefaults()
	return cfg
}
