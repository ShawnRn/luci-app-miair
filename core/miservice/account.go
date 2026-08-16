package miservice

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type QRCodeInfo struct {
	QR      string `json:"qr"`
	LP      string `json:"lp"`
	Timeout int    `json:"timeout"`
}

type DeviceInfo struct {
	DeviceID       string `json:"deviceID"`
	SerialNumber   string `json:"serialNumber"`
	Name           string `json:"name"`
	Alias          string `json:"alias"`
	Hardware       string `json:"hardware"`
	Mac            string `json:"mac"`
	CurrentLocalIP string `json:"currentLocalIP"`
}

type TokenData struct {
	UserID    string            `json:"userId"`
	PassToken string            `json:"passToken"`
	DeviceID  string            `json:"deviceId"`
	Tokens    map[string]*Token `json:"tokens"`
	Cookies   map[string]string `json:"cookies"`
}

type Token struct {
	Token     string `json:"token"`
	Ssecurity string `json:"ssecurity"`
}

type Account struct {
	Data       *TokenData
	storePath  string
	httpClient *http.Client
}

func NewAccount(storePath string) *Account {
	jar, _ := cookiejar.New(nil)
	acc := &Account{
		Data: &TokenData{
			DeviceID: randomDeviceID(),
			Tokens:   make(map[string]*Token),
			Cookies:  make(map[string]string),
		},
		storePath: storePath,
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: 15 * time.Second,
		},
	}
	acc.Load()
	return acc
}

func randomDeviceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (a *Account) Load() error {
	if a.storePath == "" {
		return nil
	}
	data, err := os.ReadFile(a.storePath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, a.Data); err != nil {
		return err
	}
	return nil
}

func (a *Account) Save() error {
	if a.storePath == "" {
		return nil
	}
	os.MkdirAll(filepath.Dir(a.storePath), 0755)
	b, err := json.MarshalIndent(a.Data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.storePath, b, 0600)
}

func (a *Account) GetQRLoginInfo() (*QRCodeInfo, error) {
	ua := fmt.Sprintf("Android-7.1.1-1.0.0-ONEPLUS A3010-136-%s APP/xiaomi.smarthome APPV/62830", a.Data.DeviceID)
	req1, _ := http.NewRequest("GET", "https://account.xiaomi.com/pass/serviceLogin?sid=mijia&_json=true", nil)
	req1.Header.Set("User-Agent", ua)
	req1.Header.Set("Cookie", fmt.Sprintf("sdkVersion=3.8.6; deviceId=%s", a.Data.DeviceID))

	resp1, err := a.httpClient.Do(req1)
	if err != nil {
		return nil, fmt.Errorf("serviceLogin request failed: %w", err)
	}
	defer resp1.Body.Close()

	body1, _ := io.ReadAll(resp1.Body)
	raw1 := strings.TrimPrefix(strings.TrimSpace(string(body1)), "&&&START&&&")

	var meta struct {
		QS       string `json:"qs"`
		Sign     string `json:"_sign"`
		Callback string `json:"callback"`
	}
	if err := json.Unmarshal([]byte(raw1), &meta); err != nil {
		return nil, fmt.Errorf("parse serviceLogin meta failed: %w", err)
	}

	nowMs := time.Now().UnixNano() / int64(time.Millisecond)
	v := url.Values{}
	v.Set("_qrsize", "240")
	v.Set("qs", meta.QS)
	v.Set("sid", "mijia")
	v.Set("_sign", meta.Sign)
	v.Set("callback", meta.Callback)
	v.Set("_json", "true")
	v.Set("_dc", fmt.Sprintf("%d", nowMs))

	loginUrlReq := "https://account.xiaomi.com/longPolling/loginUrl?" + v.Encode()
	req2, _ := http.NewRequest("GET", loginUrlReq, nil)
	req2.Header.Set("User-Agent", ua)

	resp2, err := a.httpClient.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("loginUrl request failed: %w", err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	raw2 := strings.TrimPrefix(strings.TrimSpace(string(body2)), "&&&START&&&")

	var qrResp struct {
		QR      string `json:"qr"`
		LP      string `json:"lp"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal([]byte(raw2), &qrResp); err != nil {
		return nil, fmt.Errorf("parse loginUrl response failed: %w", err)
	}

	if qrResp.QR == "" || qrResp.LP == "" {
		return nil, fmt.Errorf("invalid qr response: %s", raw2)
	}

	return &QRCodeInfo{
		QR:      qrResp.QR,
		LP:      qrResp.LP,
		Timeout: qrResp.Timeout,
	}, nil
}

func (a *Account) PollQRLogin(lpUrl string) (string, *TokenData, error) {
	req, _ := http.NewRequest("GET", lpUrl, nil)
	req.Header.Set("User-Agent", fmt.Sprintf("Android-7.1.1-1.0.0-ONEPLUS A3010-136-%s APP/xiaomi.smarthome APPV/62830", a.Data.DeviceID))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "waiting", nil, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	raw := strings.TrimPrefix(strings.TrimSpace(string(body)), "&&&START&&&")

	var pollResp struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Location    string `json:"location"`
		Nonce       int64  `json:"nonce"`
		PassToken   string `json:"passToken"`
		Ssecurity   string `json:"ssecurity"`
		UserID      int64  `json:"userId"`
	}

	if err := json.Unmarshal([]byte(raw), &pollResp); err != nil {
		return "waiting", nil, nil
	}

	if pollResp.Code == 70016 {
		return "waiting", nil, nil
	}
	if pollResp.Code == 70014 {
		return "scanned", nil, nil
	}
	if pollResp.Code == 0 && pollResp.Location != "" {
		a.Data.UserID = fmt.Sprintf("%d", pollResp.UserID)
		a.Data.PassToken = pollResp.PassToken

		err := a.securityTokenService("mijia", pollResp.Location, pollResp.Nonce, pollResp.Ssecurity)
		if err != nil {
			return "", nil, fmt.Errorf("sts mijia failed: %w", err)
		}

		_ = a.FetchServiceToken("micoapi")
		_ = a.Save()
		return "success", a.Data, nil
	}

	return fmt.Sprintf("code_%d", pollResp.Code), nil, nil
}

func (a *Account) securityTokenService(sid, location string, nonce int64, ssecurity string) error {
	nsec := fmt.Sprintf("nonce=%d&%s", nonce, ssecurity)
	h := sha1.New()
	h.Write([]byte(nsec))
	clientSign := base64.StdEncoding.EncodeToString(h.Sum(nil))

	reqUrl := fmt.Sprintf("%s&clientSign=%s", location, url.QueryEscape(clientSign))
	req, _ := http.NewRequest("GET", reqUrl, nil)
	req.Header.Set("User-Agent", fmt.Sprintf("Android-7.1.1-1.0.0-ONEPLUS A3010-136-%s APP/xiaomi.smarthome APPV/62830", a.Data.DeviceID))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	serviceToken := ""
	for _, c := range resp.Cookies() {
		a.Data.Cookies[c.Name] = c.Value
		if c.Name == "serviceToken" {
			serviceToken = c.Value
		}
	}

	if serviceToken == "" {
		return errors.New("serviceToken not found in response cookies")
	}

	a.Data.Tokens[sid] = &Token{
		Token:     serviceToken,
		Ssecurity: ssecurity,
	}

	return nil
}

func (a *Account) FetchServiceToken(sid string) error {
	if a.Data.UserID == "" || a.Data.PassToken == "" {
		return errors.New("cannot fetch serviceToken without userId and passToken")
	}

	serviceLoginURL := fmt.Sprintf("https://account.xiaomi.com/pass/serviceLogin?sid=%s&_json=true", sid)
	req, _ := http.NewRequest("GET", serviceLoginURL, nil)
	req.Header.Set("Cookie", fmt.Sprintf("userId=%s; passToken=%s", a.Data.UserID, a.Data.PassToken))
	req.Header.Set("User-Agent", fmt.Sprintf("Android-7.1.1-1.0.0-ONEPLUS A3010-136-%s APP/xiaomi.smarthome APPV/62830", a.Data.DeviceID))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	raw := strings.TrimPrefix(strings.TrimSpace(string(body)), "&&&START&&&")

	var res struct {
		Code      int    `json:"code"`
		Location  string `json:"location"`
		Nonce     int64  `json:"nonce"`
		Ssecurity string `json:"ssecurity"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return err
	}

	if res.Location == "" {
		return fmt.Errorf("serviceLogin failed for %s: code=%d", sid, res.Code)
	}

	return a.securityTokenService(sid, res.Location, res.Nonce, res.Ssecurity)
}

func (a *Account) EnsureToken(sid string) (*Token, error) {
	if tok, ok := a.Data.Tokens[sid]; ok && tok.Token != "" && tok.Ssecurity != "" {
		return tok, nil
	}
	err := a.FetchServiceToken(sid)
	if err != nil {
		return nil, err
	}
	return a.Data.Tokens[sid], nil
}

func (a *Account) RequestMina(path string, data url.Values) ([]byte, error) {
	tok, err := a.EnsureToken("micoapi")
	if err != nil {
		return nil, fmt.Errorf("ensure micoapi token failed: %w", err)
	}

	minaURL := "https://api2.mina.mi.com" + path
	var req *http.Request
	if data == nil {
		req, _ = http.NewRequest("GET", minaURL, nil)
	} else {
		req, _ = http.NewRequest("POST", minaURL, strings.NewReader(data.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	req.Header.Set("User-Agent", "MISoundBox/1.4.0, iOS/14.4")
	req.Header.Set("Cookie", fmt.Sprintf("userId=%s; serviceToken=%s", a.Data.UserID, tok.Token))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func (a *Account) DeviceList(master int) ([]DeviceInfo, error) {
	v := url.Values{}
	v.Set("master", strconv.Itoa(master))
	path := "/admin/v2/device_list?" + v.Encode()

	body, err := a.RequestMina(path, nil)
	if err != nil {
		return nil, err
	}

	var res struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Data    []DeviceInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("unmarshal device_list failed: %w, raw: %s", err, string(body))
	}

	if res.Code != 0 {
		return nil, fmt.Errorf("device_list error (code %d): %s", res.Code, res.Message)
	}

	return res.Data, nil
}

func (a *Account) PlayByMusicURL(deviceID string, streamURL string) error {
	data := url.Values{}
	data.Set("deviceId", deviceID)
	data.Set("url", streamURL)
	data.Set("type", "1")

	body, err := a.RequestMina("/music/play_by_music_url", data)
	if err != nil {
		return err
	}

	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &res); err == nil && res.Code != 0 {
		return fmt.Errorf("play_by_music_url error (code %d): %s", res.Code, res.Message)
	}

	return nil
}

func (a *Account) PlayerPause(deviceID string) error {
	data := url.Values{}
	data.Set("deviceId", deviceID)
	data.Set("type", "1")
	_, err := a.RequestMina("/player/pause", data)
	return err
}

func (a *Account) PlayerStop(deviceID string) error {
	data := url.Values{}
	data.Set("deviceId", deviceID)
	data.Set("type", "1")
	_, err := a.RequestMina("/player/stop", data)
	return err
}

func (a *Account) RequestMiIO(uri string, data interface{}) ([]byte, error) {
	tok, err := a.EnsureToken("xiaomiio")
	if err != nil {
		return nil, fmt.Errorf("ensure xiaomiio token failed: %w", err)
	}

	dataBytes, _ := json.Marshal(data)
	nonce := signNonce(tok.Ssecurity)
	signedData := signData(uri, string(dataBytes), tok.Ssecurity, nonce)

	form := url.Values{}
	form.Set("_nonce", nonce)
	form.Set("data", string(dataBytes))
	form.Set("signature", signedData)

	reqURL := "https://api.io.mi.com/app" + uri
	req, _ := http.NewRequest("POST", reqURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", fmt.Sprintf("Android-7.1.1-1.0.0-ONEPLUS A3010-136-%s APP/xiaomi.smarthome APPV/62830", a.Data.DeviceID))
	req.Header.Set("Cookie", fmt.Sprintf("userId=%s; serviceToken=%s", a.Data.UserID, tok.Token))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func signNonce(ssecurity string) string {
	b := make([]byte, 12)
	rand.Read(b)
	millis := time.Now().UnixNano() / int64(time.Millisecond) / 60000
	return fmt.Sprintf("%s:%d", base64.StdEncoding.EncodeToString(b), millis)
}

func signData(uri, data, ssecurity, nonce string) string {
	msg := fmt.Sprintf("%s&%s&%s&data=%s", uri, ssecurity, nonce, data)
	h := sha256.New()
	h.Write([]byte(msg))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func (a *Account) LoginByPassword(user, password string) error {
	signUrl := "https://account.xiaomi.com/pass/serviceLogin?sid=micoapi&_json=true"
	req, _ := http.NewRequest("GET", signUrl, nil)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	raw := strings.TrimPrefix(strings.TrimSpace(string(body)), "&&&START&&&")

	var meta struct {
		QS       string `json:"qs"`
		Sign     string `json:"_sign"`
		Callback string `json:"callback"`
		Sid      string `json:"sid"`
	}
	_ = json.Unmarshal([]byte(raw), &meta)

	pwdHash := strings.ToUpper(fmt.Sprintf("%x", md5.Sum([]byte(password))))

	form := url.Values{}
	form.Set("_json", "true")
	form.Set("qs", meta.QS)
	form.Set("sid", "micoapi")
	form.Set("_sign", meta.Sign)
	form.Set("callback", meta.Callback)
	form.Set("user", user)
	form.Set("hash", pwdHash)

	authUrl := "https://account.xiaomi.com/pass/serviceLoginAuth2"
	reqAuth, _ := http.NewRequest("POST", authUrl, strings.NewReader(form.Encode()))
	reqAuth.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqAuth.Header.Set("User-Agent", "APP/com.xiaomi.mico APPV/2.1.1")

	respAuth, err := a.httpClient.Do(reqAuth)
	if err != nil {
		return err
	}
	defer respAuth.Body.Close()

	bodyAuth, _ := io.ReadAll(respAuth.Body)
	rawAuth := strings.TrimPrefix(strings.TrimSpace(string(bodyAuth)), "&&&START&&&")

	var resAuth struct {
		Code            int    `json:"code"`
		Description     string `json:"description"`
		Location        string `json:"location"`
		Nonce           int64  `json:"nonce"`
		PassToken       string `json:"passToken"`
		Ssecurity       string `json:"ssecurity"`
		UserID          int64  `json:"userId"`
		NotificationURL string `json:"notificationUrl"`
	}
	if err := json.Unmarshal([]byte(rawAuth), &resAuth); err != nil {
		return fmt.Errorf("parse auth response failed: %v", err)
	}

	if resAuth.NotificationURL != "" || resAuth.Code == 70016 || resAuth.Code == 87001 {
		return fmt.Errorf("security_verification_required: 请使用小米/米家 App 扫码登录")
	}

	if resAuth.Location == "" {
		return fmt.Errorf("login failed (code %d): %s", resAuth.Code, resAuth.Description)
	}

	a.Data.UserID = fmt.Sprintf("%d", resAuth.UserID)
	a.Data.PassToken = resAuth.PassToken
	a.Data.Tokens["micoapi"] = &Token{
		Token:     resAuth.PassToken,
		Ssecurity: resAuth.Ssecurity,
	}

	_ = a.securityTokenService("micoapi", resAuth.Location, resAuth.Nonce, resAuth.Ssecurity)
	return a.Save()
}

