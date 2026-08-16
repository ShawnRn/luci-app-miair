package miservice

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"sort"
	"strings"
)

func MD5Hex(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}

func SHA1Base64(data []byte) string {
	h := sha1.Sum(data)
	return base64.StdEncoding.EncodeToString(h[:])
}

func SHA256Base64(data []byte) string {
	h := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(h[:])
}

func RandomHex(length int) string {
	bytes := make([]byte, length/2)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func RandomBigInt() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000000000))
	return n.String()
}

func SignNonce(ssecurity, nonce string) string {
	secBytes, _ := base64.StdEncoding.DecodeString(ssecurity)
	nonceBytes, _ := base64.StdEncoding.DecodeString(nonce)
	combined := append(secBytes, nonceBytes...)
	h := sha256.Sum256(combined)
	return base64.StdEncoding.EncodeToString(h[:])
}

func SignData(uri string, params url.Values, signNonce string) string {
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var paramParts []string
	for _, k := range keys {
		paramParts = append(paramParts, fmt.Sprintf("%s=%s", k, params.Get(k)))
	}

	raw := fmt.Sprintf("%s&%s&%s", uri, strings.Join(paramParts, "&"), signNonce)
	h := sha1.Sum([]byte(raw))
	return base64.StdEncoding.EncodeToString(h[:])
}

func DecryptRC4(cipherText []byte, password []byte) ([]byte, error) {
	c, err := rc4.NewCipher(password)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(cipherText))
	c.XORKeyStream(plain, cipherText)
	return plain, nil
}
