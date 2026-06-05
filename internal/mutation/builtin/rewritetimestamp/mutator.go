package rewritetimestamp

import (
	"context"
	"strconv"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

func init() {
	if err := mutation.Register("rewrite-timestamp", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}
	offset := 0
	if raw := cfg.Options["offset"]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			offset = parsed
		}
	}
	for i := range bundle.Timeline {
		bundle.Timeline[i].Timestamp = uint32(int(bundle.Timeline[i].Timestamp) + offset)
	}
	bundle.Metadata["mutation.rewrite-timestamp"] = strconv.Itoa(offset)
	return bundle, nil
}
