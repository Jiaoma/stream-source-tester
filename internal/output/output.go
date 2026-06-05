package output

import (
	"context"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/registry"
)

type Sink interface {
	Serve(context.Context, *model.SessionBundle, config.OutputConfig) (SessionHandle, error)
}

type Factory = registry.Factory[Sink]

var sinks = registry.New[Sink]()

func Register(name string, factory Factory) error {
	return sinks.Register(name, factory)
}

func New(name string) (Sink, error) {
	return sinks.New(name)
}

func IsRegistered(name string) bool {
	return sinks.IsRegistered(name)
}

func Registered() []string {
	return sinks.Registered()
}
