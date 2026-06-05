package output

import (
	"fmt"
	"sync"
	"time"
)

type RuntimeState string

const (
	StateCreated RuntimeState = "created"
	StateReady   RuntimeState = "ready"
	StateServing RuntimeState = "serving"
	StateStopped RuntimeState = "stopped"
	StateFailed  RuntimeState = "failed"
)

type RuntimeResult struct {
	SessionID   string
	ProfileName string
	SinkKind    string
	Target      string
	State       RuntimeState
	Streams     int
	Timeline    int
	StartedAt   time.Time
	StoppedAt   *time.Time
	Details     map[string]string
}

type SessionHandle interface {
	Result() RuntimeResult
	Close() error
}

type ManagedSession struct {
	mu     sync.RWMutex
	result RuntimeResult
}

func NewManagedSession(result RuntimeResult) *ManagedSession {
	return &ManagedSession{result: result}
}

func (s *ManagedSession) Result() RuntimeResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneResult(s.result)
}

func (s *ManagedSession) SetState(state RuntimeState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result.State = state
	if s.result.Details == nil {
		s.result.Details = map[string]string{}
	}
	s.result.Details["state"] = string(state)
}

func (s *ManagedSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.result.State == StateStopped {
		return fmt.Errorf("session %q already stopped", s.result.SessionID)
	}
	now := time.Now().UTC()
	s.result.State = StateStopped
	s.result.StoppedAt = &now
	if s.result.Details == nil {
		s.result.Details = map[string]string{}
	}
	s.result.Details["state"] = string(StateStopped)
	return nil
}

func cloneResult(value RuntimeResult) RuntimeResult {
	cloned := value
	if value.Details != nil {
		cloned.Details = make(map[string]string, len(value.Details))
		for key, item := range value.Details {
			cloned.Details[key] = item
		}
	}
	if value.StoppedAt != nil {
		stopped := *value.StoppedAt
		cloned.StoppedAt = &stopped
	}
	return cloned
}
