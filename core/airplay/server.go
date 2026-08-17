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
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	mu        sync.Mutex
	listeners map[chan []byte]bool
}

func NewAudioStreamHub() *AudioStreamHub {
	return &AudioStreamHub{
		listeners: make(map[chan []byte]bool),
	}
}

func (h *AudioStreamHub) Subscribe() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan []byte, 100)
	h.listeners[ch] = true
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
	for ch := range h.listeners {
		select {
		case ch <- data:
		default:
		}
	}
}

type Server struct {
	Name       string
	Port       int
	HTTPPort   int
	StreamPath string
	Hub        *AudioStreamHub
	OnPlay     func()
	OnStop     func()

	rsaKey  *rsa.PrivateKey
	aesKey  []byte
	aesIV   []byte
	alacDec *alac.Decoder

	serverUDP  *net.UDPConn
	rtpPort    int
	controlUDP *net.UDPConn
	ctrlPort   int
	timingUDP  *net.UDPConn
	timingPort int
	mdnsServer *zeroconf.Server
	macBytes   []byte
}

func NewServer(name string, port int, httpPort int, streamPath string) (*Server, error) {
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
		Name:       name,
		Port:       port,
		HTTPPort:   httpPort,
		StreamPath: streamPath,
		Hub:        NewAudioStreamHub(),
		rsaKey:     privKey,
		macBytes:   mac,
	}, nil
}

func (s *Server) Start() error {
	go s.startHTTPServer()

	rtspLn, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		return err
	}
	log.Printf("[AirPlay] RTSP Server listening on port %d", s.Port)

	// Audio RTP
	if rtpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0}); err == nil {
		s.serverUDP = rtpConn
		s.rtpPort = rtpConn.LocalAddr().(*net.UDPAddr).Port
		log.Printf("[AirPlay] RTP Audio listening on UDP port %d", s.rtpPort)
		go s.handleUDPPackets()
	}

	// Control UDP (RTCP)
	if ctrlConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0}); err == nil {
		s.controlUDP = ctrlConn
		s.ctrlPort = ctrlConn.LocalAddr().(*net.UDPAddr).Port
		log.Printf("[AirPlay] Control RTCP listening on UDP port %d", s.ctrlPort)
		go s.handleControlPackets()
	}

	// Timing UDP (NTP)
	if timeConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0}); err == nil {
		s.timingUDP = timeConn
		s.timingPort = timeConn.LocalAddr().(*net.UDPAddr).Port
		log.Printf("[AirPlay] Timing NTP listening on UDP port %d", s.timingPort)
		go s.handleTimingPackets()
	}

	s.registerMDNS()

	go func() {
		for {
			conn, err := rtspLn.Accept()
			if err != nil {
				break
			}
			go s.handleRTSP(conn)
		}
	}()

	return nil
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

func (s *Server) startHTTPServer() {
	mux := http.NewServeMux()
	mux.HandleFunc(s.StreamPath, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[HTTP] New stream client connected from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Connection", "close")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		w.Write(s.buildWavHeader())
		flusher, ok := w.(http.Flusher)
		if ok {
			flusher.Flush()
		}

		ch := s.Hub.Subscribe()
		defer s.Hub.Unsubscribe(ch)

		for data := range ch {
			if _, err := w.Write(data); err != nil {
				break
			}
			if ok {
				flusher.Flush()
			}
		}
	})

	log.Printf("[HTTP] Stream server listening on :%d%s", s.HTTPPort, s.StreamPath)
	http.ListenAndServe(fmt.Sprintf(":%d", s.HTTPPort), mux)
}

func (s *Server) handleRTSP(conn net.Conn) {
	defer conn.Close()
	log.Printf("[RTSP] New connection from %s", conn.RemoteAddr().String())
	reader := bufio.NewReader(conn)

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
		log.Printf("[RTSP] %s %s from %s", method, uri, conn.RemoteAddr())

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

		s.dispatchRTSP(conn, method, uri, headers, body)
	}
}

func (s *Server) dispatchRTSP(conn net.Conn, method, uri string, headers map[string]string, body []byte) {
	cseq := headers["cseq"]
	respHeaders := []string{
		"RTSP/1.0 200 OK",
		"CSeq: " + cseq,
		"Server: AirTunes/105.1",
	}

	if chall, ok := headers["apple-challenge"]; ok {
		resp := s.computeAppleResponse(chall, conn.LocalAddr())
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
		transportResp := fmt.Sprintf("RTP/AVP/UDP;unicast;mode=record;server_port=%d;control_port=%d;timing_port=%d",
			s.rtpPort, s.ctrlPort, s.timingPort)
		respHeaders = append(respHeaders, "Transport: "+transportResp)
		respHeaders = append(respHeaders, "Session: 1")
		respHeaders = append(respHeaders, "Audio-Jack-Status: connected; type=analog")
	case "RECORD":
		respHeaders = append(respHeaders, "Audio-Latency: 11025")
		if s.OnPlay != nil {
			go func() {
				// Wait for audio data to start flowing
				time.Sleep(800 * time.Millisecond)
				log.Printf("[RTSP] Triggering OnPlay callback")
				s.OnPlay()
			}()
		}
	case "FLUSH", "PAUSE":
		// Flush buffers
	case "TEARDOWN":
		if s.OnStop != nil {
			go s.OnStop()
		}
	case "SET_PARAMETER":
		// Volume / progress / metadata
	}

	resp := strings.Join(respHeaders, "\r\n") + "\r\n\r\n"
	conn.Write([]byte(resp))
}

func (s *Server) parseSDP(sdp string) {
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
			if err == nil && s.rsaKey != nil {
				// AirTunes encrypts the session AES key with RSA OAEP/SHA-1.
				// PKCS#1 v1.5 decryption cannot decode keys produced by iOS/macOS.
				key, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, s.rsaKey, encKey, nil)
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

func (s *Server) initALACDecoder(params []string) {
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

func (s *Server) handleUDPPackets() {
	buf := make([]byte, 4096)
	pktCount := 0
	for {
		n, _, err := s.serverUDP.ReadFrom(buf)
		if err != nil || n < 12 {
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
				s.Hub.Broadcast(pcm)
			}
		} else {
			// No ALAC decoder - broadcast raw
			s.Hub.Broadcast(payload)
		}
	}
}

func (s *Server) handleControlPackets() {
	buf := make([]byte, 1500)
	for {
		if s.controlUDP == nil {
			break
		}
		n, addr, err := s.controlUDP.ReadFrom(buf)
		if err != nil || n < 4 {
			continue
		}
		_ = addr
	}
}

func (s *Server) handleTimingPackets() {
	buf := make([]byte, 256)
	for {
		if s.timingUDP == nil {
			break
		}
		n, addr, err := s.timingUDP.ReadFrom(buf)
		if err != nil || n < 32 {
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
