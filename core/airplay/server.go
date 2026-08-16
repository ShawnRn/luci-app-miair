package airplay

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"miair-core/pkg_alac"
	"github.com/grandcat/zeroconf"
)

var airportPrivateKeyPEM = func() string {
	b, _ := base64.StdEncoding.DecodeString("LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcFFJQkFBS0NBUUVBNTlkRThxTGllSXRzSDFXZ2pyY0ZSS2o2ZVVXcWkrYkdMT1gxSEwzVTNHaEMvajBRZzkwdTNzRy8xQ1V0CndDNXZPWXZmRG1GSTZvU0ZYaTVFTGFiV0ptVDJkS0h6QkpLYTNrOW9rKzh0OXVjUnFNZDZEWkhKMllDQ0xsRFJLU0t2NmtEcW53NFUKd1BkcE9NWHppQy9BTWozWi9sVVZYMUc3V1NIQ0FXS2Yxek5TMWVMdnFyK2JvRWpYdUJPaXRuWi9iRHpQSHJUT1p6MERldzB1b3d4ZgovK3NHK05DSzNlUUpWeHFjYUovdkVIS0lWZDJNKzVxTDcxeUpRKzg3WDZvVjNlYVl2dDN6V1pZRDZ6NXZZVGNydGlqMlZaOVptbmkvClVBYUhxbjlKZHNCV0xVRXBWdmlZbmhpbU5WdllGWmVDWGcvSWRUUSt4NElSZGlYTnY1aEVld0lEQVFBQkFvSUJBUURsOEF4eTlYZlcKQkxta3prRWlxb1N3RjBQc21WclB6SDlLc253TEdIK1FabHZqV2Q4U1dZR043dTE1MDdIdmhGNU4zZHJKb1ZVM08xNG5EWTRURlFBYQpMbEo5Vk0zNUFBcFhhTHlZMUVSck43dTlBTEtkMkxVd1loTTdLbTUzOU80eVVGWWlrRTJuSVBzY0VzQTVsdHB4T2dVR0NZN2I3ZXo1Ck50RDZuTDFaS2F1dzdhTlhtVkF2bUpUY3VQeFdtb2t0RjNnREpLSzJ3eFp1TkdjSkUwdUZRRUc0WjNCcldQN3lvTnVTSzNkaWkyam0KbHBQSHIwTy9LblBRdHpJM2VndWhlMFR3VWVtL2VZU2R5ek15VngvWXB3a3p3dFlMM3NSNWswbzlyS1FMdHZMemZBcWRCeEJ1cmNpegphYUEvTDBISWdBbU9pdDFHSkEyc2FNeFRWUE5oQW9HQkFQZmd2MW9lWnhneG1vdGlDY01YRkVRRVdmbHpoV1lUc1hyaFVJdXo1akZ1CmEzOUdMUzk5WkVFcmhMZHJ3ajhyRERWaVJWSjVza09wOXpGdmxZQUhzMHhoOTJqaTFFN1YveXNuS0Jmc01yUGtrNUtTS1BybmpuZE0Kb1BkZXZXblZrZ0o1anhGdU5neGtPTE11RzlpNTNCNHlNdkRUQ1JpSVBNUSsrTjJpTERhUkFvR0JBTzl2Ly9tVThlVmtRYW9BTmYwWgpvTWpXOENONHh3V0EyY1NFSUhrZDlBZkZrZnR1djhveUxEQ0czWkFmMHZyaHJydGtyZmE3ZWYrQVViNjlETmdncTRtSFFBWUJwN0wrCms1REt6SnJLdU8wcitSMFliWTlwWkQxKy9nOWRWdDkxZDZMUU5lcFVFL3lZMlBQNUNOb0ZtamVkcExITU9QRmRWZ3FEekRGeFU4aEwKQW9HQkFORHJyN3hBSmJxQmpIVndJelE0VG85cGI0Qk5lcURuZGs1UWU3ZlQzKy9IMW5qR2FDMC9yWEUwUWI3cTV5U2duc0NiM0R2QQpjSnlSTTlTSjdPS2xHdDBGTVNkSkQ1S0cwWFBJcEFWTndncFhYSDVNREpnMDlLSGVoMGtYbytRQTZ2aUZCaTIxeTM0ME5vbm5FZmRmCjU0UFg0WkdTL1hhYzFVSytwTGtCQit6UkFvR0FmMEFZM0gzcUtTMmxNRUk0YnpFRm9IZUszRzg5NXBEYUszVEZCVm1EN2ZWMFpob3YKMTdmZWdGUE13T0lJOE1pc1ltOVpmVDJaMHM1Um8zczVya3QrbnZMQWRmQy9QWVBLelRMYWxwR1N3b21TTllKY0I5SE5NbG1oa0d6YwoxSm5MWVQ0aXlVeXg2cGNaQm1DZDhiRDBpd1kvRnpjZ05EYVVtYlg5K1hEdlJBMENnWUVBa0U3cElQbEU3MXF2ZkpRZ29BOWVtMGdJCkxBdUU0UHUxM2FLaUpuZmZ0N2hJamJLKzVreWIzVHlzWnZveURuYjNIT0t2SW5LN3ZYYkt1VTRJU2d4QjJiQjNIY1l6UU1Hc3oxcUoKMmdHME41aHZKcHp3d2hiaFhxRktBNHphYVNydzYyMndEbmlBSzVNbElFMHRJQUtLUDR5eE5Ham9EMlFZamhCR3VodmtXS1k9Ci0tLS0tRU5EIFJTQSBQUklWQVRFIEtFWS0tLS0t")
	return string(b)
}()

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

	rsaKey     *rsa.PrivateKey
	aesKey     []byte
	aesIV      []byte
	alacDec    *alac.Decoder

	serverUDP  *net.UDPConn
	rtpPort    int
	mdnsServer *zeroconf.Server
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

	return &Server{
		Name:       name,
		Port:       port,
		HTTPPort:   httpPort,
		StreamPath: streamPath,
		Hub:        NewAudioStreamHub(),
		rsaKey:     privKey,
	}, nil
}

func (s *Server) Start() error {
	go s.startHTTPServer()

	rtspLn, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		return err
	}
	log.Printf("[AirPlay] RTSP Server listening on port %d", s.Port)

	udpAddr, err := net.ResolveUDPAddr("udp", ":0")
	if err == nil {
		s.serverUDP, err = net.ListenUDP("udp", udpAddr)
		if err == nil {
			s.rtpPort = s.serverUDP.LocalAddr().(*net.UDPAddr).Port
			log.Printf("[AirPlay] RTP Audio listening on UDP port %d", s.rtpPort)
			go s.handleUDPPackets()
		}
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
	mac := "00:11:22:33:44:55"
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if len(iface.HardwareAddr) == 6 {
				mac = iface.HardwareAddr.String()
				break
			}
		}
	}
	rawMAC := strings.ReplaceAll(strings.ToUpper(mac), ":", "")

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

func (s *Server) startHTTPServer() {
	mux := http.NewServeMux()
	mux.HandleFunc(s.StreamPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/l16;rate=44100;channels=2")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "close")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
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
	reader := bufio.NewReader(conn)

	for {
		line, err := reader.ReadString(10)
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

		headers := make(map[string]string)
		var contentLength int
		for {
			hLine, err := reader.ReadString(10)
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
		"Server: AirTunes/220.68",
	}

	switch method {
	case "OPTIONS":
		respHeaders = append(respHeaders, "Public: ANNOUNCE, SETUP, RECORD, PAUSE, FLUSH, TEARDOWN, OPTIONS, GET_PARAMETER, SET_PARAMETER")
	case "ANNOUNCE":
		s.parseSDP(string(body))
	case "SETUP":
		respHeaders = append(respHeaders, fmt.Sprintf("Transport: RTP/AVP/UDP;unicast;interleaved=0-1;mode=record;server_port=%d;control_port=%d", s.rtpPort, s.rtpPort+1))
		respHeaders = append(respHeaders, "Session: 1")
		respHeaders = append(respHeaders, "Audio-Jack-Status: connected")
	case "RECORD":
		respHeaders = append(respHeaders, "Audio-Latency: 2205")
		if s.OnPlay != nil {
			go s.OnPlay()
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
			encKey, err := base64.StdEncoding.DecodeString(b64)
			if err == nil && s.rsaKey != nil {
				key, err := rsa.DecryptPKCS1v15(rand.Reader, s.rsaKey, encKey)
				if err == nil {
				s.aesKey = key
				log.Printf("[AirPlay] RSA AES key decoded successfully (%d bytes)", len(key))
			}
		}
	} else if strings.HasPrefix(line, "a=aesiv:") {
		b64 := strings.TrimPrefix(line, "a=aesiv:")
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
	for {
		n, _, err := s.serverUDP.ReadFrom(buf)
		if err != nil || n < 12 {
			continue
		}

		payload := buf[12:n]
		if len(payload) == 0 {
			continue
		}

		if len(s.aesKey) == 16 && len(s.aesIV) == 16 {
			block, err := aes.NewCipher(s.aesKey)
			if err == nil {
				alignedLen := (len(payload) / 16) * 16
				if alignedLen > 0 {
					mode := cipher.NewCBCDecrypter(block, s.aesIV)
					mode.CryptBlocks(payload[:alignedLen], payload[:alignedLen])
				}
			}
		}

		if s.alacDec != nil {
			pcm, err := s.alacDec.Decode(payload)
			if err == nil && len(pcm) > 0 {
				s.Hub.Broadcast(pcm)
			}
		}
	}
}
