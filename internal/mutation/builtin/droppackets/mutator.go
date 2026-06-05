package droppackets

import (
	"context"
	"strconv"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

func init() {
	if err := mutation.Register("drop-packets", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}
	dropped := len(bundle.Timeline)
	bundle.Timeline = nil
	bundle.Metadata["mutation.drop-packets"] = strconv.Itoa(dropped)
	return bundle, nil
}
