package registry

import (
	"fmt"
	"sort"
	"sync"
)

type Factory[T any] func() T

type Registry[T any] struct {
	mu        sync.RWMutex
	factories map[string]Factory[T]
}

func New[T any]() *Registry[T] {
	return &Registry[T]{factories: make(map[string]Factory[T])}
}

func (r *Registry[T]) Register(name string, factory Factory[T]) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		return fmt.Errorf("registry name must not be empty")
	}
	if factory == nil {
		return fmt.Errorf("registry factory for %q must not be nil", name)
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("registry entry %q already exists", name)
	}

	r.factories[name] = factory
	return nil
}

func (r *Registry[T]) New(name string) (T, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()

	var zero T
	if !ok {
		return zero, fmt.Errorf("registry entry %q is not registered", name)
	}

	return factory(), nil
}

func (r *Registry[T]) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.factories[name]
	return ok
}

func (r *Registry[T]) Registered() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
