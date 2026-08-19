package airplay

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/base64"
	"math/big"
	"net"
	"testing"
)

func TestComputeAppleResponsePayload(t *testing.T) {
	s, err := NewServer("test", 5000, 8300, "/stream.wav", 1500)
	if err != nil {
		t.Fatal(err)
	}
	s.macBytes = []byte{0x80, 0xaf, 0xca, 0x8c, 0x45, 0xb9}

	challenge := []byte("0123456789abcdef")
	response := s.computeAppleResponse(
		base64.RawStdEncoding.EncodeToString(challenge),
		&net.TCPAddr{IP: net.ParseIP("192.168.10.1")},
	)
	ciphertext, err := base64.RawStdEncoding.DecodeString(response)
	if err != nil {
		t.Fatal(err)
	}

	decoded := new(big.Int).Exp(
		new(big.Int).SetBytes(ciphertext),
		big.NewInt(int64(s.rsaKey.PublicKey.E)),
		s.rsaKey.PublicKey.N,
	).Bytes()
	block := make([]byte, s.rsaKey.Size())
	copy(block[len(block)-len(decoded):], decoded)

	separator := bytes.IndexByte(block[2:], 0)
	if separator < 0 {
		t.Fatal("RSA signature has no PKCS#1 separator")
	}
	payload := block[2+separator+1:]
	want := make([]byte, 32)
	copy(want, challenge)
	copy(want[16:], net.ParseIP("192.168.10.1").To4())
	copy(want[20:], s.macBytes)
	if !bytes.Equal(payload, want) {
		t.Fatalf("signed payload = %x, want %x", payload, want)
	}
}

func TestComputeAppleResponsePayloadIPv6(t *testing.T) {
	s, err := NewServer("test", 5000, 8300, "/stream.wav", 1500)
	if err != nil {
		t.Fatal(err)
	}
	s.macBytes = []byte{0x80, 0xaf, 0xca, 0x8c, 0x45, 0xb9}

	challenge := []byte("0123456789abcdef")
	ip := net.ParseIP("fdc7:f53d:f6aa::1")
	response := s.computeAppleResponse(
		base64.RawStdEncoding.EncodeToString(challenge),
		&net.TCPAddr{IP: ip},
	)
	ciphertext, err := base64.RawStdEncoding.DecodeString(response)
	if err != nil {
		t.Fatal(err)
	}

	decoded := new(big.Int).Exp(
		new(big.Int).SetBytes(ciphertext),
		big.NewInt(int64(s.rsaKey.PublicKey.E)),
		s.rsaKey.PublicKey.N,
	).Bytes()
	block := make([]byte, s.rsaKey.Size())
	copy(block[len(block)-len(decoded):], decoded)
	separator := bytes.IndexByte(block[2:], 0)
	if separator < 0 {
		t.Fatal("RSA signature has no PKCS#1 separator")
	}
	payload := block[2+separator+1:]
	want := append(append(append([]byte{}, challenge...), ip.To16()...), s.macBytes...)
	if !bytes.Equal(payload, want) {
		t.Fatalf("signed payload = %x, want %x", payload, want)
	}
}

func TestParseSDPDecryptsOAEPKey(t *testing.T) {
	s, err := NewServer("test", 5000, 8300, "/stream.wav", 1500)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("0123456789abcdef")
	encrypted, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, &s.rsaKey.PublicKey, want, nil)
	if err != nil {
		t.Fatal(err)
	}
	session := s.newSession(nil)
	defer session.Close()
	session.parseSDP("a=rsaaeskey:" + base64.RawStdEncoding.EncodeToString(encrypted))
	if !bytes.Equal(session.aesKey, want) {
		t.Fatalf("AES key = %x, want %x", session.aesKey, want)
	}
}

func TestAudioStreamHubPrebuffersAudio(t *testing.T) {
	hub := NewAudioStreamHub(1000)
	first := bytes.Repeat([]byte{0x11}, 100000)
	second := bytes.Repeat([]byte{0x22}, 100000)
	hub.Broadcast(first)
	hub.Broadcast(second)

	ch := hub.Subscribe()
	defer hub.Unsubscribe(ch)
	got := <-ch
	if !bytes.Equal(got, second) {
		t.Fatalf("prebuffer retained %d bytes with prefix %x; want latest frame", len(got), got[:1])
	}
}

func TestParseAirPlayVolume(t *testing.T) {
	tests := []struct {
		body string
		want int
	}{
		{"volume: 0.000000\r\n", 100},
		{"volume: -15.000000\r\n", 66},
		{"volume: -20.000000\r\n", 55},
		{"volume: -30.000000\r\n", 0},
		{"volume: -144.000000\r\n", 0},
	}
	for _, test := range tests {
		got, ok := parseAirPlayVolume([]byte(test.body))
		if !ok || got != test.want {
			t.Errorf("parseAirPlayVolume(%q) = %d, %v; want %d, true", test.body, got, ok, test.want)
		}
	}
	if _, ok := parseAirPlayVolume([]byte("progress: 1/2/3\r\n")); ok {
		t.Fatal("non-volume parameter was accepted")
	}
}

func TestDispatchRTSP_GetParameter(t *testing.T) {
	s, err := NewServer("test", 5000, 8300, "/stream.wav", 1500)
	if err != nil {
		t.Fatal(err)
	}
	s.GetVolume = func() int { return 70 }

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := &airplaySession{
		server: s,
		id:     "test1",
		device: "127.0.0.1:1234",
		conn:   serverConn,
		hub:    NewAudioStreamHub(1500),
	}
	defer session.hub.Close()

	go func() {
		headers := map[string]string{
			"cseq": "4",
		}
		session.dispatchRTSP("GET_PARAMETER", "rtsp://test/test1", headers, []byte("volume\r\n"))
	}()

	buf := make([]byte, 1024)
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	resp := string(buf[:n])
	if !bytes.Contains(buf[:n], []byte("RTSP/1.0 200 OK")) {
		t.Fatalf("expected 200 OK, got: %s", resp)
	}
	if !bytes.Contains(buf[:n], []byte("Content-Type: text/parameters")) {
		t.Fatalf("expected Content-Type: text/parameters, got: %s", resp)
	}
	// Speaker 70% volume maps to slider 55% = (55/100)*30 - 30 = -13.500000 dB
	if !bytes.Contains(buf[:n], []byte("volume: -13.500000")) {
		t.Fatalf("expected volume: -13.500000, got: %s", resp)
	}
}

func TestDispatchRTSP_RecordLatency(t *testing.T) {
	s, err := NewServer("test", 5000, 8300, "/stream.wav", 1500)
	if err != nil {
		t.Fatal(err)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := &airplaySession{
		server: s,
		id:     "test2",
		device: "127.0.0.1:1234",
		conn:   serverConn,
		hub:    NewAudioStreamHub(1500),
	}
	defer session.hub.Close()

	go func() {
		headers := map[string]string{
			"cseq": "5",
		}
		session.dispatchRTSP("RECORD", "rtsp://test/test2", headers, nil)
	}()

	buf := make([]byte, 1024)
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	resp := string(buf[:n])
	if !bytes.Contains(buf[:n], []byte("Audio-Latency: 4410")) {
		t.Fatalf("expected Audio-Latency: 4410, got: %s", resp)
	}
}
