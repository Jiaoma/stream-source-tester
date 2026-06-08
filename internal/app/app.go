package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	_ "stream-source-tester/internal/builtins"
	"stream-source-tester/internal/config"
	"stream-source-tester/internal/input"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
	"stream-source-tester/internal/output"
)

func Run(ctx context.Context, out io.Writer, configPath string) error {
	if configPath == "" {
		return fmt.Errorf("missing required -config path")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	plan, err := BuildPlan(cfg)
	if err != nil {
		return err
	}

	bundles, err := InstantiateProfiles(ctx, cfg, plan)
	if err != nil {
		return err
	}

	manager, err := ServeProfiles(ctx, cfg, plan, bundles)
	if err != nil {
		return err
	}
	if err := verifyRTSPListeners(manager); err != nil {
		return err
	}

	_, err = fmt.Fprintf(out,
		"loaded scenario %q with %d input(s), %d output(s), %d mutation profile(s)\nregistered inputs: %v\nregistered outputs: %v\nregistered mutators: %v\nprofiles:\n%sbundles:\n%soutputs:\n%s\nwaiting for interrupt to stop sessions...\n",
		cfg.Name,
		len(cfg.Inputs),
		len(cfg.Outputs),
		len(cfg.Mutations),
		input.Registered(),
		output.Registered(),
		mutation.Registered(),
		formatPlan(plan),
		formatBundles(bundles),
		formatManager(manager),
	)
	if err != nil {
		return err
	}

	<-ctx.Done()
	return manager.CloseAll()
}

func ServeProfiles(ctx context.Context, cfg *config.Config, plan *model.ScenarioPlan, bundles map[string]*model.SessionBundle) (*output.Manager, error) {
	indexes, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}

	manager := output.NewManager()
	for _, profile := range plan.Profiles {
		sink, err := output.New(profile.Output.Kind)
		if err != nil {
			return nil, fmt.Errorf("create output sink for profile %q: %w", profile.Name, err)
		}

		bundle, ok := bundles[profile.Name]
		if !ok {
			return nil, fmt.Errorf("bundle for profile %q not found", profile.Name)
		}

		session, err := sink.Serve(ctx, bundle, indexes.outputs[profile.Output.Name])
		if err != nil {
			return nil, fmt.Errorf("serve output for profile %q: %w", profile.Name, err)
		}
		if err := manager.Register(profile.Name, session); err != nil {
			return nil, fmt.Errorf("register output session for profile %q: %w", profile.Name, err)
		}
	}
	return manager, nil
}

func InstantiateProfiles(ctx context.Context, cfg *config.Config, plan *model.ScenarioPlan) (map[string]*model.SessionBundle, error) {
	indexes, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}

	bundles := make(map[string]*model.SessionBundle, len(plan.Profiles))
	for _, profile := range plan.Profiles {
		source, err := input.New(profile.Input.Kind)
		if err != nil {
			return nil, fmt.Errorf("create input source for profile %q: %w", profile.Name, err)
		}

		bundle, err := source.Open(ctx, indexes.inputs[profile.Input.Name])
		if err != nil {
			return nil, fmt.Errorf("open input for profile %q: %w", profile.Name, err)
		}

		bundle.Name = profile.Name
		if bundle.Metadata == nil {
			bundle.Metadata = map[string]string{}
		}
		bundle.Metadata["profile.name"] = profile.Name
		bundle.Metadata["output.name"] = profile.Output.Name
		bundle.Metadata["output.kind"] = profile.Output.Kind
		bundle.Metadata["mutation.names"] = formatMutationNames(profile.Mutations)

		bundle, err = applyConfiguredMutations(ctx, bundle, profile.Mutations, indexes.mutations)
		if err != nil {
			return nil, fmt.Errorf("apply mutations for profile %q: %w", profile.Name, err)
		}

		bundles[profile.Name] = bundle
	}

	return bundles, nil
}

func applyConfiguredMutations(ctx context.Context, bundle *model.SessionBundle, configured []model.ResolvedMutation, byName map[string]config.MutationConfig) (*model.SessionBundle, error) {
	current := bundle
	for _, item := range configured {
		if !item.Enabled {
			continue
		}

		mutator, err := mutation.New(item.Kind)
		if err != nil {
			return nil, fmt.Errorf("create mutator %q: %w", item.Name, err)
		}

		applied, err := mutator.Apply(ctx, current, byName[item.Name])
		if err != nil {
			return nil, fmt.Errorf("apply mutator %q: %w", item.Name, err)
		}
		current = applied
	}
	return current, nil
}

func BuildPlan(cfg *config.Config) (*model.ScenarioPlan, error) {
	indexes, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}

	plan := &model.ScenarioPlan{
		ScenarioName: cfg.Name,
		Profiles:     make([]model.ProfilePlan, 0, len(cfg.Profiles)),
	}

	for _, profile := range cfg.Profiles {
		resolved := model.ProfilePlan{
			Name: profile.Name,
			Input: model.ResolvedInput{
				Name:     indexes.inputs[profile.Input].Name,
				Kind:     indexes.inputs[profile.Input].Kind,
				Codec:    model.ParseCodec(indexes.inputs[profile.Input].Codec),
				Location: indexes.inputs[profile.Input].Location,
				Options:  cloneMap(indexes.inputs[profile.Input].Options),
			},
			Output: model.ResolvedOutput{
				Name:    indexes.outputs[profile.Output].Name,
				Kind:    indexes.outputs[profile.Output].Kind,
				Target:  indexes.outputs[profile.Output].Target,
				Options: cloneMap(indexes.outputs[profile.Output].Options),
			},
			Mutations: make([]model.ResolvedMutation, 0, len(profile.Mutations)),
		}

		for _, mutationName := range profile.Mutations {
			candidate := indexes.mutations[mutationName]
			resolved.Mutations = append(resolved.Mutations, model.ResolvedMutation{
				Name:    candidate.Name,
				Kind:    candidate.Kind,
				Enabled: candidate.Enabled,
				Options: cloneMap(candidate.Options),
			})
		}

		plan.Profiles = append(plan.Profiles, resolved)
	}

	return plan, nil
}

type configIndexes struct {
	inputs    map[string]config.InputConfig
	outputs   map[string]config.OutputConfig
	mutations map[string]config.MutationConfig
}

func validateConfig(cfg *config.Config) (*configIndexes, error) {
	indexes := &configIndexes{
		inputs:    make(map[string]config.InputConfig, len(cfg.Inputs)),
		outputs:   make(map[string]config.OutputConfig, len(cfg.Outputs)),
		mutations: make(map[string]config.MutationConfig, len(cfg.Mutations)),
	}

	for _, candidate := range cfg.Inputs {
		if candidate.Name == "" {
			return nil, fmt.Errorf("input name must not be empty")
		}
		if !input.IsRegistered(candidate.Kind) {
			return nil, fmt.Errorf("input kind %q is not registered", candidate.Kind)
		}
		if _, exists := indexes.inputs[candidate.Name]; exists {
			return nil, fmt.Errorf("input name %q is duplicated", candidate.Name)
		}
		indexes.inputs[candidate.Name] = candidate
	}

	for _, candidate := range cfg.Outputs {
		if candidate.Name == "" {
			return nil, fmt.Errorf("output name must not be empty")
		}
		if !output.IsRegistered(candidate.Kind) {
			return nil, fmt.Errorf("output kind %q is not registered", candidate.Kind)
		}
		if _, exists := indexes.outputs[candidate.Name]; exists {
			return nil, fmt.Errorf("output name %q is duplicated", candidate.Name)
		}
		indexes.outputs[candidate.Name] = candidate
	}

	for _, candidate := range cfg.Mutations {
		if candidate.Name == "" {
			return nil, fmt.Errorf("mutation name must not be empty")
		}
		if !mutation.IsRegistered(candidate.Kind) {
			return nil, fmt.Errorf("mutation kind %q is not registered", candidate.Kind)
		}
		if _, exists := indexes.mutations[candidate.Name]; exists {
			return nil, fmt.Errorf("mutation name %q is duplicated", candidate.Name)
		}
		indexes.mutations[candidate.Name] = candidate
	}

	for _, profile := range cfg.Profiles {
		if profile.Name == "" {
			return nil, fmt.Errorf("profile name must not be empty")
		}
		if _, ok := indexes.inputs[profile.Input]; !ok {
			return nil, fmt.Errorf("profile %q references unknown input %q", profile.Name, profile.Input)
		}
		if _, ok := indexes.outputs[profile.Output]; !ok {
			return nil, fmt.Errorf("profile %q references unknown output %q", profile.Name, profile.Output)
		}
		for _, name := range profile.Mutations {
			if _, ok := indexes.mutations[name]; !ok {
				return nil, fmt.Errorf("profile %q references unknown mutation %q", profile.Name, name)
			}
		}
	}

	return indexes, nil
}

func formatPlan(plan *model.ScenarioPlan) string {
	if len(plan.Profiles) == 0 {
		return "  (no profiles configured)\n"
	}

	var builder strings.Builder
	for _, profile := range plan.Profiles {
		builder.WriteString(fmt.Sprintf("  - %s: input=%s[%s/%s] output=%s[%s] mutations=%s\n",
			profile.Name,
			profile.Input.Name,
			profile.Input.Kind,
			profile.Input.Codec,
			profile.Output.Name,
			profile.Output.Kind,
			formatMutationNames(profile.Mutations),
		))
	}
	return builder.String()
}

func formatMutationNames(mutations []model.ResolvedMutation) string {
	if len(mutations) == 0 {
		return "none"
	}

	names := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		names = append(names, mutation.Name)
	}
	return strings.Join(names, ",")
}

func formatBundles(bundles map[string]*model.SessionBundle) string {
	if len(bundles) == 0 {
		return "  (no bundles instantiated)\n"
	}

	var builder strings.Builder
	for name, bundle := range bundles {
		streamCount := len(bundle.Streams)
		transport := make([]string, 0, len(bundle.Transport))
		for _, value := range bundle.Transport {
			transport = append(transport, string(value))
		}
		builder.WriteString(fmt.Sprintf("  - %s: streams=%d transport=%s source=%s\n",
			name,
			streamCount,
			strings.Join(transport, ","),
			bundle.Metadata["source.location"],
		))
	}
	return builder.String()
}

func formatManager(manager *output.Manager) string {
	if manager == nil {
		return "  (no outputs served)\n"
	}

	sessions := manager.List()
	if len(sessions) == 0 {
		return "  (no outputs served)\n"
	}

	var builder strings.Builder
	for name, session := range sessions {
		runtime := session.Result()
		builder.WriteString(fmt.Sprintf("  - %s: id=%s via=%s target=%s state=%s timeline=%d\n",
			name,
			runtime.SessionID,
			runtime.SinkKind,
			runtime.Target,
			runtime.State,
			runtime.Timeline,
		))
		if runtime.SinkKind == "rtsp" {
			listen := runtime.Details["listen_address"]
			mount := runtime.Details["mount_path"]
			if listen != "" && mount != "" {
				builder.WriteString(fmt.Sprintf("    open in VLC: rtsp://%s/%s\n", listen, mount))
			}
		}
	}
	return builder.String()
}

func verifyRTSPListeners(manager *output.Manager) error {
	if manager == nil {
		return nil
	}
	for profile, session := range manager.List() {
		runtime := session.Result()
		if runtime.SinkKind != "rtsp" {
			continue
		}
		listen := runtime.Details["listen_address"]
		if listen == "" {
			return fmt.Errorf("rtsp session %q has no listen address", profile)
		}
		var lastErr error
		deadline := time.Now().Add(2 * time.Second)
		for {
			conn, err := net.DialTimeout("tcp", listen, 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				break
			}
			lastErr = err
			if time.Now().After(deadline) {
				return fmt.Errorf("rtsp session %q not reachable on %s: %w", profile, listen, lastErr)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return nil
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
