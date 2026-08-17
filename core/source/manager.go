package source

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Protocol string

const (
	ProtocolAirPlay Protocol = "airplay"
	ProtocolDLNA    Protocol = "dlna"
)

type Policy string

const (
	PolicyLatest   Policy = "latest"
	PolicyLock     Policy = "lock"
	PolicyIdle     Policy = "idle"
	PolicyPriority Policy = "priority"
)

func ParsePolicy(value string) (Policy, error) {
	policy := Policy(strings.ToLower(strings.TrimSpace(value)))
	switch policy {
	case PolicyLatest, PolicyLock, PolicyIdle, PolicyPriority:
		return policy, nil
	default:
		return "", fmt.Errorf("unsupported source policy %q", value)
	}
}

type Request struct {
	ID        string
	Protocol  Protocol
	Device    string
	StreamURL string
	Cancel    func()
}

type Session struct {
	ID           string    `json:"id"`
	Protocol     Protocol  `json:"protocol"`
	Device       string    `json:"device"`
	StreamURL    string    `json:"-"`
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
	Generation   uint64    `json:"generation"`
	Cancel       func()    `json:"-"`
}

func (s Session) Public() Session {
	s.Cancel = nil
	s.StreamURL = ""
	return s
}

type Decision struct {
	Granted    bool
	Reason     string
	Session    Session
	Replaced   *Session
	Generation uint64
}

type Snapshot struct {
	Policy     Policy   `json:"policy"`
	Generation uint64   `json:"generation"`
	Active     *Session `json:"active,omitempty"`
}

type Manager struct {
	mu                sync.Mutex
	policy            Policy
	idleTimeout       time.Duration
	preferredProtocol Protocol
	generation        uint64
	active            *Session
	now               func() time.Time
}

func NewManager(policy Policy, idleTimeout time.Duration, preferred Protocol) *Manager {
	if idleTimeout <= 0 {
		idleTimeout = 10 * time.Second
	}
	if preferred != ProtocolAirPlay && preferred != ProtocolDLNA {
		preferred = ProtocolAirPlay
	}
	return &Manager{
		policy:            policy,
		idleTimeout:       idleTimeout,
		preferredProtocol: preferred,
		now:               time.Now,
	}
}

func (m *Manager) Acquire(req Request) Decision {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	if req.ID == "" {
		return Decision{Reason: "missing session id"}
	}

	if m.active != nil && m.active.ID == req.ID {
		m.active.LastActivity = now
		m.active.StreamURL = req.StreamURL
		m.active.Cancel = req.Cancel
		return Decision{Granted: true, Session: m.active.Public(), Generation: m.generation}
	}

	if m.active != nil && !m.canReplaceLocked(req.Protocol, now) {
		return Decision{
			Reason:     fmt.Sprintf("source is locked by %s session %s", m.active.Protocol, m.active.ID),
			Generation: m.generation,
		}
	}

	var replaced *Session
	if m.active != nil {
		copy := *m.active
		replaced = &copy
	}

	m.generation++
	m.active = &Session{
		ID:           req.ID,
		Protocol:     req.Protocol,
		Device:       req.Device,
		StreamURL:    req.StreamURL,
		StartedAt:    now,
		LastActivity: now,
		Generation:   m.generation,
		Cancel:       req.Cancel,
	}
	return Decision{
		Granted:    true,
		Session:    m.active.Public(),
		Replaced:   replaced,
		Generation: m.generation,
	}
}

func (m *Manager) canReplaceLocked(protocol Protocol, now time.Time) bool {
	switch m.policy {
	case PolicyLatest:
		return true
	case PolicyLock:
		return false
	case PolicyIdle:
		return now.Sub(m.active.LastActivity) >= m.idleTimeout
	case PolicyPriority:
		currentPreferred := m.active.Protocol == m.preferredProtocol
		incomingPreferred := protocol == m.preferredProtocol
		return incomingPreferred || !currentPreferred
	default:
		return true
	}
}

func (m *Manager) Touch(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil || m.active.ID != id {
		return false
	}
	m.active.LastActivity = m.now()
	return true
}

func (m *Manager) Release(id string) (uint64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil || m.active.ID != id {
		return m.generation, false
	}
	m.generation++
	m.active = nil
	return m.generation, true
}

func (m *Manager) IsOwner(id string, generation uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active != nil && m.active.ID == id && m.generation == generation
}

func (m *Manager) IsIdleGeneration(generation uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active == nil && m.generation == generation
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := Snapshot{Policy: m.policy, Generation: m.generation}
	if m.active != nil {
		copy := m.active.Public()
		snapshot.Active = &copy
	}
	return snapshot
}
