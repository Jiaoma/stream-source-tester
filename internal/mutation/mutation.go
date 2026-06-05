package mutation

import (
	"context"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/registry"
)

type Mutator interface {
	Apply(context.Context, *model.SessionBundle, config.MutationConfig) (*model.SessionBundle, error)
}

type Factory = registry.Factory[Mutator]

var mutators = registry.New[Mutator]()

func Register(name string, factory Factory) error {
	return mutators.Register(name, factory)
}

func New(name string) (Mutator, error) {
	return mutators.New(name)
}

func IsRegistered(name string) bool {
	return mutators.IsRegistered(name)
}

func Registered() []string {
	return mutators.Registered()
}
