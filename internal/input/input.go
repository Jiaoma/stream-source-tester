package input

import (
	"context"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/registry"
)

type Source interface {
	Open(context.Context, config.InputConfig) (*model.SessionBundle, error)
}

type Factory = registry.Factory[Source]

var sources = registry.New[Source]()

func Register(name string, factory Factory) error {
	return sources.Register(name, factory)
}

func New(name string) (Source, error) {
	return sources.New(name)
}

func IsRegistered(name string) bool {
	return sources.IsRegistered(name)
}

func Registered() []string {
	return sources.Registered()
}
