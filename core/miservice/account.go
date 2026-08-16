package miservice

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type DeviceInfo struct {
	DeviceID       string `json:"deviceID"`
	Hardware       string `json:"hardware"`
	Name           string `json:"name"`
	Alias          string `json:"alias"`
	CurrentLocalIP string `json:"currentLocalIP"`
	Mac            string `json:"mac"`
	MicoID         string `json:"micoID"`
}

type TokenData struct {
	Token     string `json:"token"`
	Ssecurity string `json:"ssecurity"`
}

type StoreData struct {
	UserID    string               `json:"userId"`
	PassToken string               `json:"passToken"`
	DeviceID  string               `json:"deviceId"`
	Tokens    map[string]TokenData `json:"tokens"`
}

type Account struct {
	StoreFile  string
	Data       StoreData
	httpClient *http.Client
	mu         sync.Mutex
}

func NewAccount(storeFile string) *Account {
	jar, _ := cookiejar.New(nil)
	acc := &Account{
		StoreFile: storeFile,
		Data: StoreData{
			Tokens: make(map[string]TokenData),
		},
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
	}
	acc.LoadStore()
	if acc.Data.DeviceID == "" {
		acc.Data.DeviceID = strings.ToUpper(randomHexString(8))
	}
	return acc
}

func (a *Account) LoadStore() error {
	if a.StoreFile == "" {
		return nil
	}
	data, err := os.ReadFile(a.StoreFile)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return json.Unmarshal(data, &a.Data)
}

func (a *Account) SaveStore() error {
	if a.StoreFile == "" {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	data, err := json.MarshalIndent(a.Data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.StoreFile, data, 0600)
}

type QRLoginInfo struct {
	QR      string
	LP      string
	Timeout int
}

func (a *Account) GetQRLoginInfo() (*QRLoginInfo, error) {
	apiURL := "https://account.xiaomi.com/longPolling/loginUrl?_qrsize=240&qs=%3Fsid%3Dmijia%26_json%3Dtrue&bizDeviceType=&callback=https%3A%2F%2Fsts.api.mijia.tech%2Fmijia%2Fsts&_json=true&sid=mijia"
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/6.0.103")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	raw := string(body)
	if strings.HasPrefix(raw, "&&&START&&&") {
		raw = raw[11:]
	}

	var res struct {
		Code     int    `json:"code"`
		LoginURL string `json:"loginUrl"`
		QR       string `json:"qr"`
		LP       string `json:"lp"`
		Timeout  int    `json:"timeout"`
		Desc     string `json:"desc"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, err
	}
	if res.Code != 0 || res.QR == "" {
		return nil, fmt.Errorf("failed to get qr login url: %s", res.Desc)
	}

	return &QRLoginInfo{
		QR:      res.QR,
		LP:      res.LP,
		Timeout: res.Timeout,
	}, nil
}

func (a *Account) PollQRLogin(lpURL string) (string, *StoreData, error) {
	client := &http.Client{Timeout: 40 * time.Second}
	req, _ := http.NewRequest("GET", lpURL, nil)
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/6.0.103")

	resp, err := client.Do(req)
	if err != nil {
		return "timeout", nil, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "error", nil, err
	}

	raw := string(body)
	if strings.HasPrefix(raw, "&&&START&&&") {
		raw = raw[11:]
	}

	var res struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Location    string `json:"location"`
		Nonce       int64  `json:"nonce"`
		Ssecurity   string `json:"ssecurity"`
		UserID      int64  `json:"userId"`
		PassToken   string `json:"passToken"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return "error", nil, err
	}

	if res.Code == 0 && res.PassToken != "" {
		a.Data.UserID = fmt.Sprintf("%d", res.UserID)
		a.Data.PassToken = res.PassToken
		a.SaveStore()

		// Exchange token for micoapi
		a.EnsureToken("micoapi")
		a.SaveStore()

		return "success", &a.Data, nil
	}

	return "waiting", nil, nil
}

func (a *Account) EnsureToken(sid string) (*TokenData, error) {
	if tok, ok := a.Data.Tokens[sid]; ok && tok.Token != "" {
		return &tok, nil
	}
	if a.Data.PassToken == "" || a.Data.UserID == "" {
		return nil, fmt.Errorf("no login credentials (passToken) found")
	}

	loginURL := fmt.Sprintf("https://account.xiaomi.com/pass/serviceLogin?sid=%s&_json=true", sid)
	req, _ := http.NewRequest("GET", loginURL, nil)
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/6.0.103")
	req.AddCookie(&http.Cookie{Name: "userId", Value: a.Data.UserID})
	req.AddCookie(&http.Cookie{Name: "passToken", Value: a.Data.PassToken})

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	raw := string(body)
	if strings.HasPrefix(raw, "&&&START&&&") {
		raw = raw[11:]
	}

	var res struct {
		Code      int    `json:"code"`
		Location  string `json:"location"`
		Nonce     int64  `json:"nonce"`
		Ssecurity string `json:"ssecurity"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, err
	}

	if res.Location == "" || res.Ssecurity == "" {
		return nil, fmt.Errorf("failed to get security token url for %s", sid)
	}

	nsec := fmt.Sprintf("nonce=%d&%s", res.Nonce, res.Ssecurity)
	h := sha1.New()
	h.Write([]byte(nsec))
	clientSign := base64.StdEncoding.EncodeToString(h.Sum(nil))

	secURL := fmt.Sprintf("%s&clientSign=%s", res.Location, url.QueryEscape(clientSign))
	secReq, _ := http.NewRequest("GET", secURL, nil)
	secReq.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/6.0.103")

	secResp, err := a.httpClient.Do(secReq)
	if err != nil {
		return nil, err
	}
	defer secResp.Body.Close()

	var serviceToken string
	for _, c := range secResp.Cookies() {
		if c.Name == "serviceToken" {
			serviceToken = c.Value
			break
		}
	}
	if serviceToken == "" {
		return nil, fmt.Errorf("no serviceToken cookie received")
	}

	tok := TokenData{
		Token:     serviceToken,
		Ssecurity: res.Ssecurity,
	}
	a.Data.Tokens[sid] = tok
	a.SaveStore()
	return &tok, nil
}

func (a *Account) DeviceList(master int) ([]DeviceInfo, error) {
	tok, err := a.EnsureToken("micoapi")
	if err != nil {
		return nil, fmt.Errorf("ensure micoapi token failed: %w", err)
	}

	reqURL := fmt.Sprintf("https://api2.mina.mi.com/admin/v2/device_list?master=%d", master)
	req, _ := http.NewRequest("GET", reqURL, nil)
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

	var res struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Data    []DeviceInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("mina api error (code %d): %s", res.Code, res.Message)
	}

	return res.Data, nil
}

func (a *Account) RequestMina(uri string, form url.Values) ([]byte, error) {
	tok, err := a.EnsureToken("micoapi")
	if err != nil {
		return nil, fmt.Errorf("ensure micoapi token failed: %w", err)
	}

	reqURL := "https://api2.mina.mi.com" + uri
	var req *http.Request
	if form != nil {
		req, _ = http.NewRequest("POST", reqURL, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, _ = http.NewRequest("GET", reqURL, nil)
	}

	req.Header.Set("User-Agent", "MISoundBox/1.4.0, iOS/14.4")
	req.Header.Set("Cookie", fmt.Sprintf("userId=%s; serviceToken=%s", a.Data.UserID, tok.Token))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (a *Account) PlayByMusicURL(deviceID, streamURL string) error {
	data := url.Values{}
	data.Set("deviceId", deviceID)
	
	msgJSON, _ := json.Marshal(map[string]interface{}{
		"url": streamURL,
		"type": 1,
		"media": "app_ios",
	})
	data.Set("message", string(msgJSON))
	data.Set("method", "player_play_url")
	data.Set("path", "mediaplayer")

	body, err := a.RequestMina("/remote/ubus", data)
	if err != nil {
		return err
	}

	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &res); err == nil && res.Code != 0 {
		return fmt.Errorf("player_play_url error (code %d): %s", res.Code, res.Message)
	}

	return nil
}

func (a *Account) PlayerPause(deviceID string) error {
	data := url.Values{}
	data.Set("deviceId", deviceID)
	msgJSON, _ := json.Marshal(map[string]interface{}{
		"action": "pause",
		"media": "app_ios",
	})
	data.Set("message", string(msgJSON))
	data.Set("method", "player_play_operation")
	data.Set("path", "mediaplayer")
	_, err := a.RequestMina("/remote/ubus", data)
	return err
}

func (a *Account) PlayerStop(deviceID string) error {
	data := url.Values{}
	data.Set("deviceId", deviceID)
	msgJSON, _ := json.Marshal(map[string]interface{}{
		"action": "stop",
		"media": "app_ios",
	})
	data.Set("message", string(msgJSON))
	data.Set("method", "player_play_operation")
	data.Set("path", "mediaplayer")
	_, err := a.RequestMina("/remote/ubus", data)
	return err
}

func randomHexString(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
