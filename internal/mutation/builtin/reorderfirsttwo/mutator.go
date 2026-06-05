package reorderfirsttwo

import (
	"context"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

func init() {
	if err := mutation.Register("reorder-first-two", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	_ = cfg
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}
	if len(bundle.Timeline) >= 2 {
		bundle.Timeline[0], bundle.Timeline[1] = bundle.Timeline[1], bundle.Timeline[0]
		bundle.Metadata["mutation.reorder-first-two"] = "true"
	}
	return bundle, nil
}
