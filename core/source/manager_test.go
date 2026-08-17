package source

import (
	"testing"
	"time"
)

func request(id string, protocol Protocol) Request {
	return Request{ID: id, Protocol: protocol, Device: id + "-device", StreamURL: "http://example/" + id}
}

func TestLatestPolicyPreemptsCurrentSource(t *testing.T) {
	m := NewManager(PolicyLatest, 10*time.Second, ProtocolAirPlay)
	first := m.Acquire(request("iphone", ProtocolAirPlay))
	second := m.Acquire(request("android", ProtocolDLNA))
	if !first.Granted || !second.Granted {
		t.Fatalf("latest policy did not grant both sessions: first=%+v second=%+v", first, second)
	}
	if second.Replaced == nil || second.Replaced.ID != "iphone" {
		t.Fatalf("replaced session = %+v, want iphone", second.Replaced)
	}
	if m.Snapshot().Active.ID != "android" {
		t.Fatalf("active session = %+v, want android", m.Snapshot().Active)
	}
}

func TestLockPolicyRejectsNewSource(t *testing.T) {
	m := NewManager(PolicyLock, 10*time.Second, ProtocolAirPlay)
	m.Acquire(request("iphone", ProtocolAirPlay))
	decision := m.Acquire(request("mac", ProtocolAirPlay))
	if decision.Granted {
		t.Fatal("lock policy unexpectedly granted a second source")
	}
	if m.Snapshot().Active.ID != "iphone" {
		t.Fatalf("active session = %+v, want iphone", m.Snapshot().Active)
	}
}

func TestIdlePolicyOnlyPreemptsStaleSource(t *testing.T) {
	m := NewManager(PolicyIdle, 5*time.Second, ProtocolAirPlay)
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	m.Acquire(request("iphone", ProtocolAirPlay))
	if m.Acquire(request("android", ProtocolDLNA)).Granted {
		t.Fatal("fresh source was preempted")
	}
	now = now.Add(6 * time.Second)
	if !m.Acquire(request("android", ProtocolDLNA)).Granted {
		t.Fatal("stale source was not preempted")
	}
}

func TestPriorityPolicyPrefersConfiguredProtocol(t *testing.T) {
	m := NewManager(PolicyPriority, 10*time.Second, ProtocolAirPlay)
	m.Acquire(request("android", ProtocolDLNA))
	if !m.Acquire(request("iphone", ProtocolAirPlay)).Granted {
		t.Fatal("preferred AirPlay source did not preempt DLNA")
	}
	if m.Acquire(request("android2", ProtocolDLNA)).Granted {
		t.Fatal("lower-priority DLNA source preempted AirPlay")
	}
	if !m.Acquire(request("mac", ProtocolAirPlay)).Granted {
		t.Fatal("same-priority AirPlay source should use latest-wins")
	}
}

func TestStaleReleaseDoesNotStopNewOwner(t *testing.T) {
	m := NewManager(PolicyLatest, 10*time.Second, ProtocolAirPlay)
	m.Acquire(request("iphone", ProtocolAirPlay))
	second := m.Acquire(request("mac", ProtocolAirPlay))
	if _, released := m.Release("iphone"); released {
		t.Fatal("stale session released the new owner")
	}
	if !m.IsOwner("mac", second.Generation) {
		t.Fatal("mac is no longer the active owner")
	}
}
