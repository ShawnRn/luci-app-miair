package airplay

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grandcat/zeroconf"
	"miair-core/pkg_alac"
)

var airportPrivateKeyPEM = func() string {
	b, _ := base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcFFJQkFBS0NBUUVBNTlkRThxTGllSXRzSDFXZ2pyY0ZSS2o2ZVVXcWkrYkdMT1gxSEwzVTNHaEMvajBRZzkwdTNzRy8xQ1V0CndDNXZPWXZmRG1GSTZvU0ZYaTVFTGFiV0ptVDJkS0h6QkpLYTNrOW9rKzh0OXVjUnFNZDZEWkhKMllDQ0xsRFJLU0t2NmtEcW53NFUKd1BkcE9NWHppQy9BTWozWi9sVVZYMUc3V1NIQ0FXS2Yxek5TMWVMdnFyK2JvRWpYdUJPaXRuWi9iRHpQSHJUT1p6MERldzB1b3d4ZgovK3NHK05DSzNlUUpWeHFjYUovdkVIS0lWZDJNKzVxTDcxeUpRKzg3WDZvVjNlYVl2dDN6V1pZRDZ6NXZZVGNydGlqMlZaOVptbmkvClVBYUhxbjlKZHNCV0xVRXBWdmlZbmhpbU5WdllGWmVDWGcvSWRUUSt4NElSZGlYTnY1aEVld0lEQVFBQkFvSUJBUURsOEF4eTlYZlcKQkxta3prRWlxb1N3RjBQc21WclB6SDlLc253TEdIK1FabHZqV2Q4U1dZR043dTE1MDdIdmhGNU4zZHJKb1ZVM08xNG5EWTRURlFBYQpMbEo5Vk0zNUFBcFhhTHlZMUVSck43dTlBTEtkMkxVd1loTTdLbTUzOU80eVVGWWlrRTJuSVBzY0VzQTVsdHB4T2dVR0NZN2I3ZXo1Ck50RDZuTDFaS2F1dzdhTlhtVkF2bUpUY3VQeFdtb2t0RjNnREpLSzJ3eFp1TkdjSkUwdUZRRUc0WjNCcldQN3lvTnVTSzNkaWkyam0KbHBQSHIwTy9LblBRdHpJM2VndWhlMFR3VWVtL2VZU2R5ek15VngvWXB3a3p3dFlMM3NSNWswbzlyS1FMdHZMemZBcWRCeEJ1cmNpegphYUEvTDBISWdBbU9pdDFHSkEyc2FNeFRWUE5oQW9HQkFQZmd2MW9lWnhneG1vdGlDY01YRkVRRVdmbHpoV1lUc1hyaFVJdXo1akZ1CmEzOUdMUzk5WkVFcmhMZHJ3ajhyRERWaVJWSjVza09wOXpGdmxZQUhzMHhoOTJqaTFFN1YveXNuS0Jmc01yUGtrNUtTS1BybmpuZE0Kb1BkZXZXblZrZ0o1anhGdU5neGtPTE11RzlpNTNCNHlNdkRUQ1JpSVBNUSsrTjJpTERhUkFvR0JBTzl2Ly9tVThlVmtRYW9BTmYwWgpvTWpXOENONHh3V0EyY1NFSUhrZDlBZkZrZnR1djhveUxEQ0czWkFmMHZyaHJydGtyZmE3ZWYrQVViNjlETmdncTRtSFFBWUJwN0wrCms1REt6SnJLdU8wcitSMFliWTlwWkQxKy9nOWRWdDkxZDZMUU5lcFVFL3lZMlBQNUNOb0ZtamVkcExITU9QRmRWZ3FEekRGeFU4aEwKQW9HQkFORHJyN3hBSmJxQmpIVndJelE0VG85cGI0Qk5lcURuZGs1UWU3ZlQzKy9IMW5qR2FDMC9yWEUwUWI3cTV5U2duc0NiM0R2QQpjSnlSTTlTSjdPS2xHdDBGTVNkSkQ1S0cwWFBJcEFWTndncFhYSDVNREpnMDlLSGVoMGtYbytRQTZ2aUZCaTIxeTM0ME5vbm5FZmRmCjU0UFg0WkdTL1hhYzFVSytwTGtCQit6UkFvR0FmMEFZM0gzcUtTMmxNRUk0YnpFRm9IZUszRzg5NXBEYUszVEZCVm1EN2ZWMFpob3YKMTdmZWdGUE13T0lJOE1pc1ltOVpmVDJaMHM1Um8zczVya3QrbnZMQWRmQy9QWVBLelRMYWxwR1N3b21TTllKY0I5SE5NbG1oa0d6YwoxSm5MWVQ0aXlVeXg2cGNaQm1DZDhiRDBpd1kvRnpjZ05EYVVtYlg5K1hEdlJBMENnWUVBa0U3cElQbEU3MXF2ZkpRZ29BOWVtMGdJCkxBdUU0UHUxM2FLaUpuZmZ0N2hJamJLKzVreWIzVHlzWnZveURuYjNIT0t2SW5LN3ZYYkt1VTRJU2d4QjJiQjNIY1l6UU1Hc3oxcUoKMmdHME41aHZKcHp3d2hiaFhxRktBNHphYVNydzYyMndEbmlBSzVNbElFMHRJQUtLUDR5eE5Ham9EMlFZamhCR3VodmtXS1k9Ci0tLS0tRU5EIFJTQSBQUklWQVRFIEtFWS0tLS0t")
	return string(b)
}()

const ntpEpochOffset = 2208988800

type AudioStreamHub struct {
	mu              sync.Mutex
	listeners       map[chan []byte]bool
	history         [][]byte
	historyBytes    int
	maxHistoryBytes int
	closed          bool
}

func NewAudioStreamHub(bufferMillis int) *AudioStreamHub {
	if bufferMillis < 0 {
		bufferMillis = 0
	}
	return &AudioStreamHub{
		listeners:       make(map[chan []byte]bool),
		maxHistoryBytes: 44100 * 2 * 2 * bufferMillis / 1000,
	}
}

func (h *AudioStreamHub) Subscribe() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		ch := make(chan []byte)
		close(ch)
		return ch
	}
	// Size the listener queue to hold the complete pre-buffer plus enough room
	// for live frames while the HTTP writer catches up.
	ch := make(chan []byte, len(h.history)+128)
	h.listeners[ch] = true
	for _, frame := range h.history {
		ch <- frame
	}
	return ch
}

func (h *AudioStreamHub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.listeners[ch]; ok {
		delete(h.listeners, ch)
		close(ch)
	}
}

func (h *AudioStreamHub) Broadcast(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	if h.maxHistoryBytes > 0 {
		frame := append([]byte(nil), data...)
		h.history = append(h.history, frame)
		h.historyBytes += len(frame)
		for h.historyBytes > h.maxHistoryBytes && len(h.history) > 0 {
			h.historyBytes -= len(h.history[0])
			h.history[0] = nil
			h.history = h.history[1:]
		}
	}
	for ch := range h.listeners {
		select {
		case ch <- data:
		default:
		}
	}
}

func (h *AudioStreamHub) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.history = nil
	h.historyBytes = 0
}

func (h *AudioStreamHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	h.history = nil
	h.historyBytes = 0
	for ch := range h.listeners {
		delete(h.listeners, ch)
		close(ch)
	}
}

type SessionInfo struct {
	ID         string
	Device     string
	StreamPath string
	Cancel     func()
}

type Server struct {
	Name         string
	Port         int
	HTTPPort     int
	StreamPath   string
	BufferMillis int

	OnSessionStart    func(SessionInfo) bool
	OnSessionStop     func(sessionID string)
	OnSessionActivity func(sessionID string)
	OnVolume          func(sessionID string, volume int)

	rsaKey     *rsa.PrivateKey
	mdnsServer *zeroconf.Server
	macBytes   []byte

	sessionsMu sync.RWMutex
	sessions   map[string]*airplaySession
	nextID     atomic.Uint64
	rtspLn     net.Listener
	httpServer *http.Server
	httpLn     net.Listener
}

type airplaySession struct {
	server *Server
	id     string
	device string
	conn   net.Conn
	hub    *AudioStreamHub

	mu      sync.Mutex
	aesKey  []byte
	aesIV   []byte
	alacDec *alac.Decoder
	owned   bool

	rtpUDP     *net.UDPConn
	rtpPort    int
	controlUDP *net.UDPConn
	ctrlPort   int
	timingUDP  *net.UDPConn
	timingPort int
	closeOnce  sync.Once
}

func NewServer(name string, port int, httpPort int, streamPath string, bufferMillis int) (*Server, error) {
	block, _ := pem.Decode([]byte(airportPrivateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	mac := []byte{0x80, 0xAF, 0xCA, 0x8C, 0x45, 0xB8}
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if len(iface.HardwareAddr) == 6 && (iface.Flags&net.FlagLoopback == 0) {
				mac = iface.HardwareAddr
				break
			}
		}
	}

	return &Server{
		Name:         name,
		Port:         port,
		HTTPPort:     httpPort,
		StreamPath:   strings.TrimSuffix(streamPath, ".wav"),
		BufferMillis: bufferMillis,
		rsaKey:       privKey,
		macBytes:     mac,
		sessions:     make(map[string]*airplaySession),
	}, nil
}

func (s *Server) Start() error {
	if err := s.startHTTPServer(); err != nil {
		return err
	}

	rtspLn, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		_ = s.httpServer.Close()
		return err
	}
	s.rtspLn = rtspLn
	log.Printf("[AirPlay] RTSP Server listening on port %d", s.Port)

	s.registerMDNS()

	go func() {
		for {
			conn, err := rtspLn.Accept()
			if err != nil {
				break
			}
			session := s.newSession(conn)
			go session.handleRTSP()
		}
	}()

	return nil
}

func (s *Server) Close() {
	if s.rtspLn != nil {
		_ = s.rtspLn.Close()
	}
	if s.httpServer != nil {
		_ = s.httpServer.Close()
	}
	if s.mdnsServer != nil {
		s.mdnsServer.Shutdown()
	}

	s.sessionsMu.RLock()
	sessions := make([]*airplaySession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.sessionsMu.RUnlock()
	for _, session := range sessions {
		session.Close()
	}
}

func (s *Server) newSession(conn net.Conn) *airplaySession {
	id := fmt.Sprintf("a%x", s.nextID.Add(1))
	device := "unknown"
	if conn != nil && conn.RemoteAddr() != nil {
		device = conn.RemoteAddr().String()
	}
	session := &airplaySession{
		server: s,
		id:     id,
		device: device,
		conn:   conn,
		hub:    NewAudioStreamHub(s.BufferMillis),
	}
	s.sessionsMu.Lock()
	s.sessions[id] = session
	s.sessionsMu.Unlock()
	return session
}

func (s *Server) session(id string) *airplaySession {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	return s.sessions[id]
}

func (s *Server) removeSession(id string) {
	s.sessionsMu.Lock()
	delete(s.sessions, id)
	s.sessionsMu.Unlock()
}

func (s *Server) registerMDNS() {
	rawMAC := fmt.Sprintf("%02X%02X%02X%02X%02X%02X",
		s.macBytes[0], s.macBytes[1], s.macBytes[2],
		s.macBytes[3], s.macBytes[4], s.macBytes[5])

	txt := []string{
		"txtvers=1",
		"ch=2",
		"cn=0,1",
		"et=0,1",
		"sv=false",
		"da=true",
		"sr=44100",
		"ss=16",
		"pw=false",
		"vn=65537",
		"tp=TCP,UDP",
		"md=0,1,2",
		"am=" + s.Name,
		"sf=0x4",
	}

	server, err := zeroconf.Register(
		fmt.Sprintf("%s@%s", rawMAC, s.Name),
		"_raop._tcp",
		"local.",
		s.Port,
		txt,
		nil,
	)
	if err != nil {
		log.Printf("[mDNS] Failed to register _raop._tcp: %v", err)
	} else {
		s.mdnsServer = server
		log.Printf("[mDNS] Registered AirPlay service: %s (%s)", s.Name, rawMAC)
	}
}

func (s *Server) computeAppleResponse(challB64 string, localAddr net.Addr) string {
	challB64 = strings.TrimSpace(challB64)
	for len(challB64)%4 != 0 {
		challB64 += "="
	}
	challBytes, err := base64.StdEncoding.DecodeString(challB64)
	if err != nil {
		return ""
	}
	if len(challBytes) > 16 {
		log.Printf("[RTSP] Ignoring oversized Apple-Challenge (%d bytes)", len(challBytes))
		return ""
	}
	var receiverIP []byte
	if tcpAddr, ok := localAddr.(*net.TCPAddr); ok && tcpAddr.IP != nil {
		if ip4 := tcpAddr.IP.To4(); ip4 != nil {
			receiverIP = ip4
		} else {
			receiverIP = tcpAddr.IP.To16()
		}
	}
	if receiverIP == nil {
		receiverIP = []byte{192, 168, 10, 1}
	}

	mac6 := make([]byte, 6)
	copy(mac6, s.macBytes)

	// RAOP authenticates the receiver by signing challenge || receiver IP ||
	// receiver MAC, padded to a minimum of 32 bytes. With IPv6 this is 38 bytes
	// and must not be truncated. Padding the challenge itself, or truncating the
	// IPv6 form, produces a response that Apple clients reject.
	signedData := make([]byte, 0, 38)
	signedData = append(signedData, challBytes...)
	signedData = append(signedData, receiverIP...)
	signedData = append(signedData, mac6...)
	if len(signedData) < 32 {
		signedData = append(signedData, make([]byte, 32-len(signedData))...)
	}

	k := s.rsaKey.Size()
	msg := make([]byte, k)
	msg[0] = 0x00
	msg[1] = 0x01
	padLen := k - 3 - len(signedData)
	for i := 0; i < padLen; i++ {
		msg[2+i] = 0xFF
	}
	msg[2+padLen] = 0x00
	copy(msg[3+padLen:], signedData)

	c := new(big.Int).SetBytes(msg)
	m := new(big.Int).Exp(c, s.rsaKey.D, s.rsaKey.N)
	mBytes := m.Bytes()
	if len(mBytes) < k {
		paddedM := make([]byte, k)
		copy(paddedM[k-len(mBytes):], mBytes)
		mBytes = paddedM
	}

	respB64 := base64.StdEncoding.EncodeToString(mBytes)
	for len(respB64) > 0 && respB64[len(respB64)-1] == '=' {
		respB64 = respB64[:len(respB64)-1]
	}
	return respB64
}

func (s *Server) buildWavHeader() []byte {
	h := make([]byte, 44)
	copy(h[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(h[4:8], 0x7FFFFF00+36)
	copy(h[8:12], []byte("WAVE"))
	copy(h[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(h[16:20], 16)
	binary.LittleEndian.PutUint16(h[20:22], 1)
	binary.LittleEndian.PutUint16(h[22:24], 2)
	binary.LittleEndian.PutUint32(h[24:28], 44100)
	binary.LittleEndian.PutUint32(h[28:32], 44100*2*2)
	binary.LittleEndian.PutUint16(h[32:34], 4)
	binary.LittleEndian.PutUint16(h[34:36], 16)
	copy(h[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(h[40:44], 0x7FFFFF00)
	return h
}

func (s *Server) startHTTPServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.StreamPath+"/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, s.StreamPath+"/")
		id := strings.TrimSuffix(name, ".wav")
		session := s.session(id)
		if session == nil || name == id {
			http.NotFound(w, r)
			return
		}
		log.Printf("[HTTP] Stream client connected to AirPlay session %s from %s", id, r.RemoteAddr)
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Connection", "close")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Accept-Ranges", "none")

		if _, err := w.Write(s.buildWavHeader()); err != nil {
			return
		}
		flusher, ok := w.(http.Flusher)
		if ok {
			flusher.Flush()
		}

		ch := session.hub.Subscribe()
		defer session.hub.Unsubscribe(ch)

		for data := range ch {
			if _, err := w.Write(data); err != nil {
				break
			}
			// Drain all currently available buffered frames without flushing per-packet
		drainLoop:
			for {
				select {
				case nextData, ok := <-ch:
					if !ok {
						break drainLoop
					}
					if _, err := w.Write(nextData); err != nil {
						return
					}
				default:
					break drainLoop
				}
			}
			if ok {
				flusher.Flush()
			}
		}
	})

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.HTTPPort))
	if err != nil {
		return err
	}
	s.httpLn = listener
	s.httpServer = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("[HTTP] AirPlay stream server listening on :%d%s/{session}.wav", s.HTTPPort, s.StreamPath)
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[HTTP] AirPlay stream server stopped: %v", err)
		}
	}()
	return nil
}

func (s *airplaySession) handleRTSP() {
	if s.conn == nil {
		return
	}
	defer s.Close()
	log.Printf("[RTSP] New AirPlay session %s from %s", s.id, s.device)
	reader := bufio.NewReader(s.conn)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, " ")
		if len(parts) < 3 {
			continue
		}
		method := parts[0]
		uri := parts[1]
		log.Printf("[RTSP] Session %s: %s %s from %s", s.id, method, uri, s.device)

		headers := make(map[string]string)
		var contentLength int
		for {
			hLine, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			hLine = strings.TrimRight(hLine, "\r\n")
			if hLine == "" {
				break
			}
			idx := strings.Index(hLine, ":")
			if idx > 0 {
				k := strings.ToLower(strings.TrimSpace(hLine[:idx]))
				v := strings.TrimSpace(hLine[idx+1:])
				headers[k] = v
				if k == "content-length" {
					contentLength, _ = strconv.Atoi(v)
				}
			}
		}

		var body []byte
		if contentLength > 0 {
			body = make([]byte, contentLength)
			io.ReadFull(reader, body)
		}

		if !s.dispatchRTSP(method, uri, headers, body) {
			return
		}
	}
}

func (s *airplaySession) dispatchRTSP(method, uri string, headers map[string]string, body []byte) bool {
	cseq := headers["cseq"]
	respHeaders := []string{
		"RTSP/1.0 200 OK",
		"CSeq: " + cseq,
		"Server: AirTunes/105.1",
	}

	if chall, ok := headers["apple-challenge"]; ok {
		resp := s.server.computeAppleResponse(chall, s.conn.LocalAddr())
		if resp != "" {
			respHeaders = append(respHeaders, "Apple-Response: "+resp)
		}
	}

	switch method {
	case "OPTIONS":
		respHeaders = append(respHeaders, "Public: ANNOUNCE, SETUP, RECORD, PAUSE, FLUSH, TEARDOWN, OPTIONS, GET_PARAMETER, SET_PARAMETER")
	case "ANNOUNCE":
		s.parseSDP(string(body))
	case "SETUP":
		if err := s.setupUDP(); err != nil {
			respHeaders[0] = "RTSP/1.0 500 Internal Server Error"
			break
		}
		transportResp := fmt.Sprintf("RTP/AVP/UDP;unicast;mode=record;server_port=%d;control_port=%d;timing_port=%d",
			s.rtpPort, s.ctrlPort, s.timingPort)
		respHeaders = append(respHeaders, "Transport: "+transportResp)
		respHeaders = append(respHeaders, "Session: "+s.id)
		respHeaders = append(respHeaders, "Audio-Jack-Status: connected; type=analog")
	case "RECORD":
		if !s.activate() {
			respHeaders[0] = "RTSP/1.0 453 Not Enough Bandwidth"
			break
		}
		respHeaders = append(respHeaders, "Audio-Latency: 11025")
	case "FLUSH":
		s.hub.Reset()
	case "PAUSE":
		s.hub.Reset()
		s.releaseOwnership()
	case "TEARDOWN":
		// The response is written before the deferred Close releases resources.
	case "SET_PARAMETER":
		if volume, ok := parseAirPlayVolume(body); ok && s.server.OnVolume != nil {
			s.server.OnVolume(s.id, volume)
			s.touch()
		}
	}

	resp := strings.Join(respHeaders, "\r\n") + "\r\n\r\n"
	if _, err := s.conn.Write([]byte(resp)); err != nil {
		return false
	}
	return method != "TEARDOWN"
}

func parseAirPlayVolume(body []byte) (int, bool) {
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "volume") {
			continue
		}
		db, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return 0, false
		}
		// RAOP uses 0 dB as full volume, roughly -30 dB as the audible
		// minimum, and -144 dB as the explicit mute value.
		if db <= -144 {
			return 0, true
		}
		if db < -30 {
			db = -30
		}
		if db > 0 {
			db = 0
		}
		return int(math.Round((db + 30) / 30 * 100)), true
	}
	return 0, false
}

func (s *airplaySession) streamPath() string {
	return fmt.Sprintf("%s/%s.wav", s.server.StreamPath, s.id)
}

func (s *airplaySession) activate() bool {
	s.mu.Lock()
	alreadyOwned := s.owned
	s.mu.Unlock()
	if alreadyOwned {
		if s.server.OnSessionActivity != nil {
			s.server.OnSessionActivity(s.id)
		}
		return true
	}

	granted := true
	if s.server.OnSessionStart != nil {
		granted = s.server.OnSessionStart(SessionInfo{ID: s.id, Device: s.device, StreamPath: s.streamPath(), Cancel: s.Close})
	}
	if granted {
		s.mu.Lock()
		s.owned = true
		s.mu.Unlock()
		log.Printf("[RTSP] AirPlay session %s acquired the speaker", s.id)
	}
	return granted
}

func (s *airplaySession) releaseOwnership() {
	s.mu.Lock()
	owned := s.owned
	s.owned = false
	s.mu.Unlock()
	if owned && s.server.OnSessionStop != nil {
		s.server.OnSessionStop(s.id)
	}
}

func (s *airplaySession) Close() {
	s.closeOnce.Do(func() {
		s.releaseOwnership()
		if s.conn != nil {
			_ = s.conn.Close()
		}
		if s.rtpUDP != nil {
			_ = s.rtpUDP.Close()
		}
		if s.controlUDP != nil {
			_ = s.controlUDP.Close()
		}
		if s.timingUDP != nil {
			_ = s.timingUDP.Close()
		}
		s.hub.Close()
		s.server.removeSession(s.id)
		log.Printf("[RTSP] AirPlay session %s closed", s.id)
	})
}

func (s *airplaySession) setupUDP() error {
	if s.rtpUDP != nil && s.controlUDP != nil && s.timingUDP != nil {
		return nil
	}

	rtpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	controlConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		_ = rtpConn.Close()
		return err
	}
	timingConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		_ = rtpConn.Close()
		_ = controlConn.Close()
		return err
	}

	s.rtpUDP = rtpConn
	s.rtpPort = rtpConn.LocalAddr().(*net.UDPAddr).Port
	s.controlUDP = controlConn
	s.ctrlPort = controlConn.LocalAddr().(*net.UDPAddr).Port
	s.timingUDP = timingConn
	s.timingPort = timingConn.LocalAddr().(*net.UDPAddr).Port
	log.Printf("[AirPlay] Session %s ports: RTP=%d RTCP=%d timing=%d", s.id, s.rtpPort, s.ctrlPort, s.timingPort)
	go s.handleUDPPackets()
	go s.handleControlPackets()
	go s.handleTimingPackets()
	return nil
}

func (s *airplaySession) parseSDP(sdp string) {
	for _, line := range strings.Split(sdp, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "a=rsaaeskey:") {
			b64 := strings.TrimPrefix(line, "a=rsaaeskey:")
			b64 = strings.ReplaceAll(b64, " ", "")
			b64 = strings.ReplaceAll(b64, "\r", "")
			for len(b64)%4 != 0 {
				b64 += "="
			}
			encKey, err := base64.StdEncoding.DecodeString(b64)
			if err == nil && s.server.rsaKey != nil {
				// AirTunes encrypts the session AES key with RSA OAEP/SHA-1.
				// PKCS#1 v1.5 decryption cannot decode keys produced by iOS/macOS.
				key, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, s.server.rsaKey, encKey, nil)
				if err == nil {
					s.aesKey = key
					log.Printf("[AirPlay] RSA AES key decoded successfully (%d bytes)", len(key))
				}
			}
		} else if strings.HasPrefix(line, "a=aesiv:") {
			b64 := strings.TrimPrefix(line, "a=aesiv:")
			b64 = strings.ReplaceAll(b64, " ", "")
			b64 = strings.ReplaceAll(b64, "\r", "")
			for len(b64)%4 != 0 {
				b64 += "="
			}
			iv, err := base64.StdEncoding.DecodeString(b64)
			if err == nil {
				s.aesIV = iv
				log.Printf("[AirPlay] AES IV decoded successfully (%d bytes)", len(iv))
			}
		} else if strings.HasPrefix(line, "a=fmtp:") {
			parts := strings.Split(line, " ")
			if len(parts) > 1 {
				s.initALACDecoder(parts[1:])
			}
		}
	}
}

func (s *airplaySession) initALACDecoder(params []string) {
	var ints []int
	for _, p := range params {
		val, err := strconv.Atoi(p)
		if err == nil {
			ints = append(ints, val)
		}
	}
	if len(ints) >= 11 {
		cookie := make([]byte, 24)
		binary.BigEndian.PutUint32(cookie[0:4], uint32(ints[0]))
		cookie[4] = byte(ints[1])
		cookie[5] = byte(ints[2])
		cookie[6] = byte(ints[3])
		cookie[7] = byte(ints[4])
		cookie[8] = byte(ints[5])
		cookie[9] = byte(ints[6])
		binary.BigEndian.PutUint16(cookie[10:12], uint16(ints[7]))
		binary.BigEndian.PutUint32(cookie[12:16], uint32(ints[8]))
		binary.BigEndian.PutUint32(cookie[16:20], uint32(ints[9]))
		binary.BigEndian.PutUint32(cookie[20:24], uint32(ints[10]))

		dec, err := alac.New(cookie)
		if err == nil {
			s.alacDec = dec
			log.Printf("[AirPlay] ALAC decoder initialized (%d Hz, %d ch, %d bit)", ints[10], ints[6], ints[2])
		}
	}
}

func (s *airplaySession) handleUDPPackets() {
	buf := make([]byte, 4096)
	pktCount := 0
	for {
		n, _, err := s.rtpUDP.ReadFrom(buf)
		if err != nil {
			return
		}
		if n < 12 {
			continue
		}

		payload := buf[12:n]
		if len(payload) == 0 {
			continue
		}

		pktCount++
		if pktCount == 1 {
			log.Printf("[RTP] First audio packet received (%d bytes payload)", len(payload))
		} else if pktCount%500 == 0 {
			log.Printf("[RTP] Received %d audio packets", pktCount)
		}

		if len(s.aesKey) == 16 && len(s.aesIV) == 16 {
			block, err := aes.NewCipher(s.aesKey)
			if err == nil {
				alignedLen := (len(payload) / 16) * 16
				if alignedLen > 0 {
					// Fresh IV copy each time - CBC modifies IV in place
					iv := make([]byte, 16)
					copy(iv, s.aesIV)
					mode := cipher.NewCBCDecrypter(block, iv)
					mode.CryptBlocks(payload[:alignedLen], payload[:alignedLen])
				}
			}
		}

		if s.alacDec != nil {
			pcm, err := s.alacDec.Decode(payload)
			if err != nil {
				if pktCount <= 5 {
					log.Printf("[ALAC] Decode error on packet %d: %v", pktCount, err)
				}
				continue
			}
			if len(pcm) > 0 {
				if pktCount == 1 {
					log.Printf("[ALAC] First decoded frame: %d bytes PCM", len(pcm))
				}
				s.hub.Broadcast(pcm)
				s.touch()
			}
		} else {
			// No ALAC decoder - broadcast raw
			s.hub.Broadcast(payload)
			s.touch()
		}
	}
}

func (s *airplaySession) touch() {
	if s.server.OnSessionActivity != nil {
		s.server.OnSessionActivity(s.id)
	}
}

func (s *airplaySession) handleControlPackets() {
	buf := make([]byte, 1500)
	for {
		if s.controlUDP == nil {
			break
		}
		n, addr, err := s.controlUDP.ReadFrom(buf)
		if err != nil {
			return
		}
		if n < 4 {
			continue
		}
		_ = addr
	}
}

func (s *airplaySession) handleTimingPackets() {
	buf := make([]byte, 256)
	for {
		if s.timingUDP == nil {
			break
		}
		n, addr, err := s.timingUDP.ReadFrom(buf)
		if err != nil {
			return
		}
		if n < 32 {
			continue
		}

		ptype := buf[1] & 0x7F
		if ptype == 0x52 {
			now := time.Now().UnixNano()
			sec := uint32(now/1e9 + ntpEpochOffset)
			frac := uint32((float64(now%1e9) / 1e9) * 4294967296.0)

			resp := make([]byte, 32)
			resp[0] = 0x80
			resp[1] = 0xD3
			copy(resp[2:4], buf[2:4])
			copy(resp[4:12], buf[24:32])
			binary.BigEndian.PutUint32(resp[12:16], sec)
			binary.BigEndian.PutUint32(resp[16:20], frac)
			binary.BigEndian.PutUint32(resp[20:24], sec)
			binary.BigEndian.PutUint32(resp[24:28], frac)

			s.timingUDP.WriteTo(resp, addr)
		}
	}
}
