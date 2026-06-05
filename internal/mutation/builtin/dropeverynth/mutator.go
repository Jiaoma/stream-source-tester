package dropeverynth

import (
	"context"
	"strconv"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

func init() {
	if err := mutation.Register("drop-every-nth", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}
	n := 0
	if raw := cfg.Options["n"]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			n = parsed
		}
	}
	if n <= 0 {
		return bundle, nil
	}
	filtered := make([]model.PacketEvent, 0, len(bundle.Timeline))
	dropped := 0
	for i, event := range bundle.Timeline {
		if (i+1)%n == 0 {
			dropped++
			continue
		}
		filtered = append(filtered, event)
	}
	bundle.Timeline = filtered
	bundle.Metadata["mutation.drop-every-nth"] = strconv.Itoa(dropped)
	return bundle, nil
}
