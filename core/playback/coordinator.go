package playback

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"miair-core/miservice"
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
	manager       *source.Manager
	speaker       Speaker
	deviceID      string
	statusPath    string
	volumePath    string
	volumeMu      sync.RWMutex
	currentVolume int
	commands      chan command
	volumeUpdates chan command
	done          chan struct{}
	closeOnce     sync.Once
}

type RuntimeStatus struct {
	UpdatedAt time.Time              `json:"updated_at"`
	Source    source.Snapshot        `json:"source"`
	Token     *miservice.TokenStatus `json:"token,omitempty"`
}

func NewCoordinator(manager *source.Manager, speaker Speaker, deviceID, statusPath, volumePath string) *Coordinator {
	initialVolume := 50
	if volumePath != "" {
		if data, err := os.ReadFile(volumePath); err == nil {
			var v int
			if _, err := fmt.Sscanf(string(data), "%d", &v); err == nil && v >= 0 && v <= 100 {
				initialVolume = v
			}
		}
	}

	c := &Coordinator{
		manager:       manager,
		speaker:       speaker,
		deviceID:      deviceID,
		statusPath:    statusPath,
		volumePath:    volumePath,
		currentVolume: initialVolume,
		commands:      make(chan command, 32),
		volumeUpdates: make(chan command, 16),
		done:          make(chan struct{}),
	}
	go c.controlLoop()
	go c.volumeWorker()
	go c.statusLoop()
	return c
}

func (c *Coordinator) GetVolume() int {
	c.volumeMu.RLock()
	defer c.volumeMu.RUnlock()
	if c.currentVolume < 0 {
		return 0
	}
	if c.currentVolume > 100 {
		return 100
	}
	return c.currentVolume
}

func (c *Coordinator) saveVolume(vol int) {
	if c.volumePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.volumePath), 0o755); err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.tmp.%d", c.volumePath, time.Now().UnixNano())
	if err := os.WriteFile(tmp, []byte(fmt.Sprintf("%d\n", vol)), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, c.volumePath)
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

	// Queue playback start directly into high-priority command queue
	c.enqueue(command{
		kind:       commandPlay,
		sessionID:  req.ID,
		generation: decision.Generation,
		streamURL:  req.StreamURL,
	})
	return decision
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

	c.volumeMu.Lock()
	c.currentVolume = volume
	c.volumeMu.Unlock()
	c.saveVolume(volume)

	select {
	case c.volumeUpdates <- command{
		kind:       commandVolume,
		sessionID:  sessionID,
		generation: snapshot.Generation,
		volume:     volume,
	}:
	case <-c.done:
	}
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
		case <-c.done:
			return
		}
	}
}

func (c *Coordinator) volumeWorker() {
	var (
		pending *command
		timer   *time.Timer
		timerCh <-chan time.Time
	)

	applyPending := func() {
		if pending == nil || c.speaker == nil || c.deviceID == "" {
			return
		}
		cmd := *pending
		pending = nil
		if c.manager.IsOwner(cmd.sessionID, cmd.generation) {
			if err := c.speaker.PlayerSetVolume(c.deviceID, cmd.volume); err != nil {
				log.Printf("[Source] Speaker volume update failed: %v", err)
			} else {
				log.Printf("[Source] Speaker volume set to %d by session %s", cmd.volume, cmd.sessionID)
			}
		}
	}

	for {
		select {
		case cmd := <-c.volumeUpdates:
			pending = &cmd
			if timer == nil {
				timer = time.NewTimer(100 * time.Millisecond)
				timerCh = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(100 * time.Millisecond)
			}
		case <-timerCh:
			applyPending()
		case <-c.done:
			if timer != nil {
				timer.Stop()
			}
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
				log.Printf("[Source] Speaker started session %s on attempt %d (url: %s)", cmd.sessionID, attempt, cmd.streamURL)
				vol := c.GetVolume()
				go func(sid string, gen uint64, v int) {
					time.Sleep(500 * time.Millisecond)
					if c.manager.IsOwner(sid, gen) {
						if err := c.speaker.PlayerSetVolume(c.deviceID, v); err != nil {
							log.Printf("[Source] Initial volume sync failed: %v", err)
						} else {
							log.Printf("[Source] Initial speaker volume synced to remembered %d%%", v)
						}
					}
				}(cmd.sessionID, cmd.generation, vol)
				return
			}
			log.Printf("[Source] Speaker play attempt %d failed for session %s (url: %s): %v", attempt, cmd.sessionID, cmd.streamURL, err)
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
	status := RuntimeStatus{
		UpdatedAt: time.Now(),
		Source:    c.manager.Snapshot(),
	}
	if tsp, ok := c.speaker.(interface{ GetTokenStatus() miservice.TokenStatus }); ok {
		ts := tsp.GetTokenStatus()
		status.Token = &ts
	}
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
