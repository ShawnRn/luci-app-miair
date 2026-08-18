package dlna

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ssdpAddress = "239.255.255.250:1900"
	serverToken = "Linux/5.10 UPnP/1.1 MiAir/1.1"
)

type SessionInfo struct {
	ID        string
	Device    string
	StreamURL string
	Cancel    func()
}

type mediaSession struct {
	id        string
	remoteURL string
	ctx       context.Context
	cancel    context.CancelFunc
}

type Server struct {
	Name   string
	HostIP string
	Port   int
	UDN    string

	OnSessionStart    func(SessionInfo) bool
	OnSessionStop     func(sessionID string)
	OnSessionActivity func(sessionID string)
	OnVolume          func(sessionID string, volume int)

	mu              sync.RWMutex
	currentURI      string
	currentMetadata string
	transportState  string
	volume          int
	muted           bool
	activeSession   string
	startedAt       time.Time
	media           map[string]*mediaSession
	nextID          atomic.Uint64

	httpServer *http.Server
	ssdpConn   *net.UDPConn
	done       chan struct{}
	closeOnce  sync.Once
	client     *http.Client
}

func NewServer(name, hostIP string, port int) *Server {
	sum := sha1.Sum([]byte("miair-dlna:" + hostIP + ":" + name))
	hexID := hex.EncodeToString(sum[:16])
	udn := fmt.Sprintf("uuid:%s-%s-%s-%s-%s", hexID[:8], hexID[8:12], hexID[12:16], hexID[16:20], hexID[20:32])
	return &Server{
		Name:           name,
		HostIP:         hostIP,
		Port:           port,
		UDN:            udn,
		transportState: "NO_MEDIA_PRESENT",
		volume:         50,
		media:          make(map[string]*mediaSession),
		done:           make(chan struct{}),
		client: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				ForceAttemptHTTP2:     false,
				MaxIdleConns:          16,
				IdleConnTimeout:       30 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
			},
		},
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/device.xml", s.handleDeviceDescription)
	mux.HandleFunc("/upnp/avtransport.xml", s.handleAVTransportSCPD)
	mux.HandleFunc("/upnp/renderingcontrol.xml", s.handleRenderingControlSCPD)
	mux.HandleFunc("/upnp/connectionmanager.xml", s.handleConnectionManagerSCPD)
	mux.HandleFunc("/upnp/control/avtransport", s.handleAVTransport)
	mux.HandleFunc("/upnp/control/renderingcontrol", s.handleRenderingControl)
	mux.HandleFunc("/upnp/control/connectionmanager", s.handleConnectionManager)
	mux.HandleFunc("/upnp/event/", s.handleEventSubscription)
	mux.HandleFunc("/media/", s.handleMediaProxy)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		return err
	}
	s.httpServer = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[DLNA] HTTP server stopped: %v", err)
		}
	}()

	if err := s.startSSDP(); err != nil {
		_ = s.httpServer.Close()
		return err
	}
	log.Printf("[DLNA] MediaRenderer %q listening on http://%s:%d", s.Name, s.HostIP, s.Port)
	return nil
}

func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.mu.Lock()
		for _, media := range s.media {
			media.cancel()
		}
		s.media = make(map[string]*mediaSession)
		s.mu.Unlock()
		s.sendNotify("ssdp:byebye")
		if s.ssdpConn != nil {
			_ = s.ssdpConn.Close()
		}
		if s.httpServer != nil {
			_ = s.httpServer.Close()
		}
	})
}

func (s *Server) handleDeviceDescription(w http.ResponseWriter, _ *http.Request) {
	writeXML(w, http.StatusOK, deviceDescription(s.Name, s.UDN))
}

func (s *Server) handleAVTransportSCPD(w http.ResponseWriter, _ *http.Request) {
	writeXML(w, http.StatusOK, avTransportSCPD)
}

func (s *Server) handleRenderingControlSCPD(w http.ResponseWriter, _ *http.Request) {
	writeXML(w, http.StatusOK, renderingControlSCPD)
}

func (s *Server) handleConnectionManagerSCPD(w http.ResponseWriter, _ *http.Request) {
	writeXML(w, http.StatusOK, connectionManagerSCPD)
}

func (s *Server) handleEventSubscription(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "SUBSCRIBE":
		sid := r.Header.Get("SID")
		if sid == "" {
			sid = s.UDN + "-event"
		}
		w.Header().Set("SID", sid)
		w.Header().Set("TIMEOUT", "Second-1800")
		w.WriteHeader(http.StatusOK)
	case "UNSUBSCRIBE":
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) newMediaSession(remoteURL string) *mediaSession {
	id := fmt.Sprintf("d%x", s.nextID.Add(1))
	ctx, cancel := context.WithCancel(context.Background())
	media := &mediaSession{id: id, remoteURL: remoteURL, ctx: ctx, cancel: cancel}
	s.mu.Lock()
	s.media[id] = media
	s.mu.Unlock()
	return media
}

func (s *Server) cancelMedia(id string, notify bool) {
	s.mu.Lock()
	media := s.media[id]
	if media != nil {
		media.cancel()
		delete(s.media, id)
	}
	wasActive := s.activeSession == id
	if wasActive {
		s.activeSession = ""
		s.transportState = "STOPPED"
	}
	s.mu.Unlock()
	if notify && wasActive && s.OnSessionStop != nil {
		s.OnSessionStop(id)
	}
}

func (s *Server) handleMediaProxy(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/media/")
	s.mu.RLock()
	media := s.media[id]
	s.mu.RUnlock()
	if media == nil || id == "" {
		log.Printf("[DLNA] Media session %s not found (URL: %s from %s)", id, r.URL.Path, r.RemoteAddr)
		http.NotFound(w, r)
		return
	}

	log.Printf("[DLNA] Media client connected for session %s (method: %s, from %s, remoteURL: %s)", id, r.Method, r.RemoteAddr, media.remoteURL)

	request, err := http.NewRequestWithContext(media.ctx, r.Method, media.remoteURL, nil)
	if err != nil {
		log.Printf("[DLNA] Failed to create request for %s: %v", media.remoteURL, err)
		http.Error(w, "invalid media URL", http.StatusBadGateway)
		return
	}
	for _, header := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match"} {
		if value := r.Header.Get(header); value != "" {
			request.Header.Set(header, value)
		}
	}
	request.Header.Set("User-Agent", serverToken)
	response, err := s.client.Do(request)
	if err != nil {
		log.Printf("[DLNA] Upstream request failed for session %s (%s): %v", id, media.remoteURL, err)
		http.Error(w, "media source unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()

	log.Printf("[DLNA] Upstream responded for session %s: status=%d, type=%s, length=%s", id, response.StatusCode, response.Header.Get("Content-Type"), response.Header.Get("Content-Length"))

	for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead {
		return
	}

	flusher, hasFlusher := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, rErr := response.Body.Read(buf)
		if n > 0 {
			if _, wErr := w.Write(buf[:n]); wErr != nil {
				log.Printf("[DLNA] Client write failed for session %s: %v", id, wErr)
				break
			}
			if hasFlusher {
				flusher.Flush()
			}
			if s.OnSessionActivity != nil {
				s.OnSessionActivity(id)
			}
		}
		if rErr != nil {
			if rErr != io.EOF {
				log.Printf("[DLNA] Upstream read finished for session %s: %v", id, rErr)
			}
			break
		}
	}
}

func (s *Server) startSSDP() error {
	group, err := net.ResolveUDPAddr("udp4", ssdpAddress)
	if err != nil {
		return err
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return err
	}
	_ = conn.SetReadBuffer(64 * 1024)
	s.ssdpConn = conn
	go s.ssdpLoop()
	go s.notifyLoop()
	s.sendNotify("ssdp:alive")
	return nil
}

func (s *Server) ssdpLoop() {
	buffer := make([]byte, 4096)
	for {
		_ = s.ssdpConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, addr, err := s.ssdpConn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				select {
				case <-s.done:
					return
				default:
					continue
				}
			}
			return
		}
		message := string(buffer[:n])
		if !strings.HasPrefix(strings.ToUpper(message), "M-SEARCH ") || !strings.Contains(strings.ToLower(message), "ssdp:discover") {
			continue
		}
		st := headerValue(message, "ST")
		for _, target := range s.searchTargets() {
			if st == "ssdp:all" || st == target.st {
				s.sendSearchResponse(addr, target.st, target.usn)
			}
		}
	}
}

type searchTarget struct {
	st  string
	usn string
}

func (s *Server) searchTargets() []searchTarget {
	return []searchTarget{
		{st: "upnp:rootdevice", usn: s.UDN + "::upnp:rootdevice"},
		{st: s.UDN, usn: s.UDN},
		{st: "urn:schemas-upnp-org:device:MediaRenderer:1", usn: s.UDN + "::urn:schemas-upnp-org:device:MediaRenderer:1"},
		{st: avTransportURN, usn: s.UDN + "::" + avTransportURN},
		{st: renderingControlURN, usn: s.UDN + "::" + renderingControlURN},
		{st: connectionManagerURN, usn: s.UDN + "::" + connectionManagerURN},
	}
}

func (s *Server) sendSearchResponse(addr *net.UDPAddr, st, usn string) {
	message := strings.Join([]string{
		"HTTP/1.1 200 OK",
		"CACHE-CONTROL: max-age=1800",
		"EXT:",
		"LOCATION: " + s.location(),
		"SERVER: " + serverToken,
		"ST: " + st,
		"USN: " + usn,
		"BOOTID.UPNP.ORG: 1",
		"CONFIGID.UPNP.ORG: 1",
		"", "",
	}, "\r\n")
	_, _ = s.ssdpConn.WriteToUDP([]byte(message), addr)
}

func (s *Server) notifyLoop() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sendNotify("ssdp:alive")
		case <-s.done:
			return
		}
	}
}

func (s *Server) sendNotify(nts string) {
	addr, err := net.ResolveUDPAddr("udp4", ssdpAddress)
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()
	for _, target := range s.searchTargets() {
		message := strings.Join([]string{
			"NOTIFY * HTTP/1.1",
			"HOST: " + ssdpAddress,
			"CACHE-CONTROL: max-age=1800",
			"LOCATION: " + s.location(),
			"NT: " + target.st,
			"NTS: " + nts,
			"SERVER: " + serverToken,
			"USN: " + target.usn,
			"BOOTID.UPNP.ORG: 1",
			"CONFIGID.UPNP.ORG: 1",
			"", "",
		}, "\r\n")
		_, _ = conn.Write([]byte(message))
	}
}

func (s *Server) location() string {
	return fmt.Sprintf("http://%s:%d/device.xml", s.HostIP, s.Port)
}

func headerValue(message, name string) string {
	for _, line := range strings.Split(message, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], name) {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

func validateMediaURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported media URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("media URL has no host")
	}
	return nil
}

func durationString(elapsed time.Duration) string {
	if elapsed < 0 {
		elapsed = 0
	}
	total := int(elapsed.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", total/3600, (total/60)%60, total%60)
}

func parseVolume(value string) int {
	volume, err := strconv.Atoi(value)
	if err != nil {
		return 50
	}
	if volume < 0 {
		return 0
	}
	if volume > 100 {
		return 100
	}
	return volume
}
