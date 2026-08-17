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
	s, err := NewServer("test", 5000, 8300, "/stream.wav")
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
	s, err := NewServer("test", 5000, 8300, "/stream.wav")
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
	s, err := NewServer("test", 5000, 8300, "/stream.wav")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("0123456789abcdef")
	encrypted, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, &s.rsaKey.PublicKey, want, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.parseSDP("a=rsaaeskey:" + base64.RawStdEncoding.EncodeToString(encrypted))
	if !bytes.Equal(s.aesKey, want) {
		t.Fatalf("AES key = %x, want %x", s.aesKey, want)
	}
}
