package playback

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"miair-core/source"
)

type Speaker interface {
	PlayByMusicURL(deviceID, streamURL string) error
	PlayerPause(deviceID string) error
	PlayerSetVolume(deviceID string, volume int) error
}

type commandKind int

const (
	commandPlay commandKind = iota
	commandPause
	commandVolume
)

type command struct {
	kind       commandKind
	sessionID  string
	generation uint64
	streamURL  string
	volume     int
}

type Coordinator struct {
	manager    *source.Manager
	speaker    Speaker
	deviceID   string
	statusPath string
	commands   chan command
	volumes    chan command
	done       chan struct{}
	closeOnce  sync.Once
}

type RuntimeStatus struct {
	UpdatedAt time.Time       `json:"updated_at"`
	Source    source.Snapshot `json:"source"`
}

func NewCoordinator(manager *source.Manager, speaker Speaker, deviceID, statusPath string) *Coordinator {
	c := &Coordinator{
		manager:    manager,
		speaker:    speaker,
		deviceID:   deviceID,
		statusPath: statusPath,
		commands:   make(chan command, 32),
		volumes:    make(chan command, 1),
		done:       make(chan struct{}),
	}
	go c.controlLoop()
	go c.statusLoop()
	return c
}

func (c *Coordinator) Activate(req source.Request) source.Decision {
	decision := c.manager.Acquire(req)
	if !decision.Granted {
		log.Printf("[Source] Rejected %s session %s from %s: %s", req.Protocol, req.ID, req.Device, decision.Reason)
		return decision
	}

	if decision.Replaced != nil {
		log.Printf("[Source] %s session %s preempted %s session %s", req.Protocol, req.ID, decision.Replaced.Protocol, decision.Replaced.ID)
		if decision.Replaced.Cancel != nil {
			decision.Replaced.Cancel()
		}
	} else {
		log.Printf("[Source] Activated %s session %s from %s", req.Protocol, req.ID, req.Device)
	}

	c.enqueueVolume(command{
		kind:       commandPlay,
		sessionID:  req.ID,
		generation: decision.Generation,
		streamURL:  req.StreamURL,
	})
	return decision
}

// enqueueVolume coalesces rapid slider updates so the control loop always
// applies the newest value without blocking the AirPlay RTSP connection.
func (c *Coordinator) enqueueVolume(cmd command) {
	select {
	case c.volumes <- cmd:
		return
	default:
	}
	select {
	case <-c.volumes:
	default:
	}
	select {
	case c.volumes <- cmd:
	case <-c.done:
	}
}

func (c *Coordinator) Deactivate(sessionID string) bool {
	generation, released := c.manager.Release(sessionID)
	if !released {
		return false
	}
	log.Printf("[Source] Released session %s", sessionID)
	c.enqueue(command{kind: commandPause, sessionID: sessionID, generation: generation})
	return true
}

func (c *Coordinator) Touch(sessionID string) bool {
	return c.manager.Touch(sessionID)
}

func (c *Coordinator) SetVolume(sessionID string, volume int) bool {
	snapshot := c.manager.Snapshot()
	if snapshot.Active == nil || snapshot.Active.ID != sessionID {
		return false
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	c.enqueue(command{
		kind:       commandVolume,
		sessionID:  sessionID,
		generation: snapshot.Generation,
		volume:     volume,
	})
	return true
}

func (c *Coordinator) Snapshot() source.Snapshot {
	return c.manager.Snapshot()
}

func (c *Coordinator) Close() {
	c.closeOnce.Do(func() { close(c.done) })
}

func (c *Coordinator) enqueue(cmd command) {
	select {
	case c.commands <- cmd:
	case <-c.done:
	}
}

func (c *Coordinator) controlLoop() {
	for {
		select {
		case cmd := <-c.commands:
			c.handleCommand(cmd)
		case cmd := <-c.volumes:
			c.handleCommand(cmd)
		case <-c.done:
			return
		}
	}
}

func (c *Coordinator) handleCommand(cmd command) {
	if c.speaker == nil || c.deviceID == "" {
		return
	}

	switch cmd.kind {
	case commandPlay:
		for attempt := 1; attempt <= 3; attempt++ {
			if !c.manager.IsOwner(cmd.sessionID, cmd.generation) {
				return
			}
			err := c.speaker.PlayByMusicURL(c.deviceID, cmd.streamURL)
			if err == nil {
				log.Printf("[Source] Speaker started session %s on attempt %d", cmd.sessionID, attempt)
				return
			}
			log.Printf("[Source] Speaker play attempt %d failed for session %s: %v", attempt, cmd.sessionID, err)
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}
	case commandPause:
		if c.manager.IsIdleGeneration(cmd.generation) {
			if err := c.speaker.PlayerPause(c.deviceID); err != nil {
				log.Printf("[Source] Speaker pause failed: %v", err)
			}
		}
	case commandVolume:
		if c.manager.IsOwner(cmd.sessionID, cmd.generation) {
			if err := c.speaker.PlayerSetVolume(c.deviceID, cmd.volume); err != nil {
				log.Printf("[Source] Speaker volume update failed: %v", err)
			} else {
				log.Printf("[Source] Speaker volume set to %d by session %s", cmd.volume, cmd.sessionID)
			}
		}
	}
}

func (c *Coordinator) statusLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	c.writeStatus()
	for {
		select {
		case <-ticker.C:
			c.writeStatus()
		case <-c.done:
			c.writeStatus()
			return
		}
	}
}

func (c *Coordinator) writeStatus() {
	if c.statusPath == "" {
		return
	}
	status := RuntimeStatus{UpdatedAt: time.Now(), Source: c.manager.Snapshot()}
	data, err := json.Marshal(status)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.statusPath), 0o755); err != nil {
		return
	}
	tmpPath := c.statusPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmpPath, c.statusPath)
}
