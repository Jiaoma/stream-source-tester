package output

import (
	"testing"
	"time"
)

func TestManagerRegisterLookupAndClose(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	session := NewManagedSession(RuntimeResult{
		SessionID:   "rtsp:profile-a",
		ProfileName: "profile-a",
		SinkKind:    "rtsp",
		Target:      "rtsp://127.0.0.1:8554/test",
		State:       StateReady,
		StartedAt:   time.Now().UTC(),
	})

	if err := manager.Register("profile-a", session); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got, ok := manager.Get("profile-a"); !ok || got == nil {
		t.Fatalf("Get(profile-a) failed")
	}
	if got, ok := manager.GetBySessionID("rtsp:profile-a"); !ok || got == nil {
		t.Fatalf("GetBySessionID(rtsp:profile-a) failed")
	}
	if len(manager.List()) != 1 {
		t.Fatalf("List() length = %d, want 1", len(manager.List()))
	}
	if err := manager.Close("profile-a"); err != nil {
		t.Fatalf("Close(profile-a) error = %v", err)
	}
	closed := session.Result()
	if closed.State != StateStopped {
		t.Fatalf("closed state = %q, want %q", closed.State, StateStopped)
	}
	if closed.StoppedAt == nil {
		t.Fatalf("closed StoppedAt should be set")
	}
}

func TestManagerCloseAll(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	for _, profile := range []string{"profile-a", "profile-b"} {
		session := NewManagedSession(RuntimeResult{
			SessionID:   "rtsp:" + profile,
			ProfileName: profile,
			SinkKind:    "rtsp",
			Target:      "rtsp://127.0.0.1:8554/" + profile,
			State:       StateReady,
			StartedAt:   time.Now().UTC(),
		})
		if err := manager.Register(profile, session); err != nil {
			t.Fatalf("Register(%s) error = %v", profile, err)
		}
	}

	if err := manager.CloseAll(); err != nil {
		t.Fatalf("CloseAll() error = %v", err)
	}
	for profile, session := range manager.List() {
		if session.Result().State != StateStopped {
			t.Fatalf("session %s state = %q, want %q", profile, session.Result().State, StateStopped)
		}
	}
}

func TestManagerListShowsServingState(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	session := NewManagedSession(RuntimeResult{
		SessionID:   "rtp-udp:profile-a",
		ProfileName: "profile-a",
		SinkKind:    "rtp-udp",
		Target:      "127.0.0.1:5004",
		State:       StateServing,
		StartedAt:   time.Now().UTC(),
	})
	if err := manager.Register("profile-a", session); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	listed := manager.List()
	got, ok := listed["profile-a"]
	if !ok {
		t.Fatalf("profile-a not found in list")
	}
	if got.Result().State != StateServing {
		t.Fatalf("listed session state = %q, want %q", got.Result().State, StateServing)
	}
}
