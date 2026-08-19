package playback

import (
	"sync"
	"testing"
	"time"

	"miair-core/source"
)

type fakeSpeaker struct {
	mu      sync.Mutex
	plays   []string
	pauses  int
	volumes []int
}

func (f *fakeSpeaker) PlayByMusicURL(_ string, streamURL string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.plays = append(f.plays, streamURL)
	return nil
}

func (f *fakeSpeaker) PlayerPause(_ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pauses++
	return nil
}

func (f *fakeSpeaker) PlayerSetVolume(_ string, volume int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.volumes = append(f.volumes, volume)
	return nil
}

func (f *fakeSpeaker) snapshot() ([]string, int, []int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.plays...), f.pauses, append([]int(nil), f.volumes...)
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestCoordinatorIgnoresStaleStopAfterPreemption(t *testing.T) {
	manager := source.NewManager(source.PolicyLatest, 10*time.Second, source.ProtocolAirPlay)
	speaker := &fakeSpeaker{}
	coordinator := NewCoordinator(manager, speaker, "speaker", "", "")
	defer coordinator.Close()

	coordinator.Activate(source.Request{ID: "iphone", Protocol: source.ProtocolAirPlay, StreamURL: "http://iphone"})
	coordinator.Activate(source.Request{ID: "mac", Protocol: source.ProtocolAirPlay, StreamURL: "http://mac"})
	coordinator.Deactivate("iphone")

	waitFor(t, func() bool {
		plays, _, _ := speaker.snapshot()
		return len(plays) > 0 && plays[len(plays)-1] == "http://mac"
	})
	_, pauses, _ := speaker.snapshot()
	if pauses != 0 {
		t.Fatalf("stale stop paused the speaker %d times", pauses)
	}
}

func TestCoordinatorPausesWhenCurrentOwnerStops(t *testing.T) {
	manager := source.NewManager(source.PolicyLatest, 10*time.Second, source.ProtocolAirPlay)
	speaker := &fakeSpeaker{}
	coordinator := NewCoordinator(manager, speaker, "speaker", "", "")
	defer coordinator.Close()

	coordinator.Activate(source.Request{ID: "iphone", Protocol: source.ProtocolAirPlay, StreamURL: "http://iphone"})
	waitFor(t, func() bool {
		plays, _, _ := speaker.snapshot()
		return len(plays) == 1
	})
	coordinator.Deactivate("iphone")
	waitFor(t, func() bool {
		_, pauses, _ := speaker.snapshot()
		return pauses == 1
	})
}

func TestCoordinatorOnlyChangesVolumeForOwner(t *testing.T) {
	manager := source.NewManager(source.PolicyLatest, 10*time.Second, source.ProtocolAirPlay)
	speaker := &fakeSpeaker{}
	coordinator := NewCoordinator(manager, speaker, "speaker", "", "")
	defer coordinator.Close()

	coordinator.Activate(source.Request{ID: "android", Protocol: source.ProtocolDLNA, StreamURL: "http://android"})
	if coordinator.SetVolume("other", 90) {
		t.Fatal("non-owner changed volume")
	}
	if !coordinator.SetVolume("android", 120) {
		t.Fatal("owner could not change volume")
	}
	waitFor(t, func() bool {
		_, _, volumes := speaker.snapshot()
		return len(volumes) == 1
	})
	_, _, volumes := speaker.snapshot()
	if volumes[0] != 100 {
		t.Fatalf("volume = %d, want 100", volumes[0])
	}
	if coordinator.GetVolume() != 100 {
		t.Fatalf("GetVolume() = %d, want 100", coordinator.GetVolume())
	}
}

func TestCoordinatorVolumePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	volPath := tmpDir + "/volume"

	manager := source.NewManager(source.PolicyLatest, 10*time.Second, source.ProtocolAirPlay)
	speaker := &fakeSpeaker{}
	coordinator := NewCoordinator(manager, speaker, "speaker", "", volPath)
	if coordinator.GetVolume() != 50 {
		t.Fatalf("initial volume = %d, want default 50", coordinator.GetVolume())
	}

	coordinator.Activate(source.Request{ID: "session1", Protocol: source.ProtocolAirPlay, StreamURL: "http://test"})
	coordinator.SetVolume("session1", 75)
	if coordinator.GetVolume() != 75 {
		t.Fatalf("updated volume = %d, want 75", coordinator.GetVolume())
	}
	coordinator.Close()

	// Reopen with new coordinator to test persistence
	manager2 := source.NewManager(source.PolicyLatest, 10*time.Second, source.ProtocolAirPlay)
	speaker2 := &fakeSpeaker{}
	coordinator2 := NewCoordinator(manager2, speaker2, "speaker", "", volPath)
	defer coordinator2.Close()

	if coordinator2.GetVolume() != 75 {
		t.Fatalf("restored volume = %d, want 75", coordinator2.GetVolume())
	}
}
