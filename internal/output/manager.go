package output

import "fmt"

type Manager struct {
	byProfile map[string]SessionHandle
	byID      map[string]SessionHandle
}

func NewManager() *Manager {
	return &Manager{
		byProfile: map[string]SessionHandle{},
		byID:      map[string]SessionHandle{},
	}
}

func (m *Manager) Register(profile string, session SessionHandle) error {
	if profile == "" {
		return fmt.Errorf("profile name must not be empty")
	}
	if session == nil {
		return fmt.Errorf("session must not be nil")
	}
	if _, exists := m.byProfile[profile]; exists {
		return fmt.Errorf("profile %q already registered", profile)
	}

	result := session.Result()
	if result.SessionID == "" {
		return fmt.Errorf("session for profile %q must have a session id", profile)
	}
	if _, exists := m.byID[result.SessionID]; exists {
		return fmt.Errorf("session id %q already registered", result.SessionID)
	}

	m.byProfile[profile] = session
	m.byID[result.SessionID] = session
	return nil
}

func (m *Manager) Get(profile string) (SessionHandle, bool) {
	session, ok := m.byProfile[profile]
	return session, ok
}

func (m *Manager) GetBySessionID(id string) (SessionHandle, bool) {
	session, ok := m.byID[id]
	return session, ok
}

func (m *Manager) List() map[string]SessionHandle {
	cloned := make(map[string]SessionHandle, len(m.byProfile))
	for key, value := range m.byProfile {
		cloned[key] = value
	}
	return cloned
}

func (m *Manager) Close(profile string) error {
	session, ok := m.byProfile[profile]
	if !ok {
		return fmt.Errorf("profile %q is not registered", profile)
	}
	return session.Close()
}

func (m *Manager) CloseAll() error {
	for profile, session := range m.byProfile {
		if err := session.Close(); err != nil {
			if err.Error() == fmt.Sprintf("session %q already stopped", session.Result().SessionID) {
				continue
			}
			return fmt.Errorf("close profile %q: %w", profile, err)
		}
	}
	return nil
}
