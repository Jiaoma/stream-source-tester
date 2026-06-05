package setmarker

import (
	"context"
	"strconv"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

func init() {
	if err := mutation.Register("set-marker", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}

	value := true
	if raw := cfg.Options["value"]; raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err == nil {
			value = parsed
		}
	}

	for i := range bundle.Timeline {
		bundle.Timeline[i].Marker = value
	}
	bundle.Metadata["mutation.set-marker"] = strconv.FormatBool(value)
	return bundle, nil
}
