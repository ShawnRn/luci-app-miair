
package miservice

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

type QRCodeSession struct {
	LoginURL string `json:"loginUrl"`
	QR       string `json:"qr"`
	LP       string `json:"lp"`
	Timeout  int    `json:"timeout"`
}

type Account struct {
	UserID    string            `json:"userId"`
	PassToken string            `json:"passToken"`
	DeviceID  string            `json:"deviceId"`
	Tokens    map[string]*Token `json:"tokens"`
	Cookies   map[string]string `json:"cookies"`

	storePath  string
	httpClient *http.Client
}

type Token struct {
	Token     string `json:"token"`
	Ssecurity string `json:"ssecurity"`
}

func NewAccount(storePath string) *Account {
	jar, _ := cookiejar.New(nil)
	acc := &Account{
		Tokens:    make(map[string]*Token),
		Cookies:   make(map[string]string),
		storePath: storePath,
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: 15 * time.Second,
		},
	}
	acc.load()
	return acc
}

func (a *Account) load() error {
	if a.storePath == "" {
		return nil
	}
	b, err := os.ReadFile(a.storePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, a)
}

func (a *Account) Save() error {
	if a.storePath == "" {
		return nil
	}
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.storePath, b, 0600)
}

func (a *Account) RequestQRCode() (*QRCodeSession, error) {
	signUrl := "https://account.xiaomi.com/longPolling/loginUrl?_qrsize=240&qs=%3Fsid%3Dmijia%26_json%3Dtrue&bizDeviceType=&callback=https%3A%2F%2Fsts.api.mijia.tech%2Fmijia%2Fsts&_json=true&sid=mijia"
	req, _ := http.NewRequest("GET", signUrl, nil)
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/6.0.103")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	raw := string(body)
	if strings.HasPrefix(raw, "&&&START&&&") {
		raw = raw[11:]
	}

	var qr QRCodeSession
	if err := json.Unmarshal([]byte(raw), &qr); err != nil {
		return nil, fmt.Errorf("decode qr failed: %v", err)
	}
	return &qr, nil
}

func (a *Account) PollQRCode(lp string) (string, error) {
	req, _ := http.NewRequest("GET", lp, nil)
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/6.0.103")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	raw := string(body)
	if strings.HasPrefix(raw, "&&&START&&&") {
		raw = raw[11:]
	}

	var res struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Location    string `json:"location"`
		Nonce       int64  `json:"nonce"`
		PassToken   string `json:"passToken"`
		Ssecurity   string `json:"ssecurity"`
		UserID      int64  `json:"userId"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return "", err
	}
	if res.Code == 70016 {
		return "waiting", nil
	}
	if res.Code == 70014 {
		return "scanned", nil
	}
	if res.Code == 0 && res.Location != "" {
		if err := a.handleLoginLocation(res.Location, res.Nonce, res.Ssecurity, res.PassToken, fmt.Sprintf("%d", res.UserID)); err != nil {
			return "", err
		}
		a.Save()
		return "success", nil
	}
	return fmt.Sprintf("code_%d", res.Code), nil
}

func (a *Account) handleLoginLocation(location string, nonce int64, ssecurity, passToken, userId string) error {
	a.UserID = userId
	a.PassToken = passToken

	req, _ := http.NewRequest("GET", location, nil)
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/6.0.103")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for _, c := range resp.Cookies() {
		a.Cookies[c.Name] = c.Value
		if c.Name == "serviceToken" {
			a.Tokens["mijia"] = &Token{
				Token:     c.Value,
				Ssecurity: ssecurity,
			}
		}
	}
	return nil
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
	raw := string(body)
	if strings.HasPrefix(raw, "&&&START&&&") {
		raw = raw[11:]
	}

	var meta struct {
		QS    string `json:"qs"`
		Sign  string `json:"_sign"`
		Callback string `json:"callback"`
		Sid   string `json:"sid"`
	}
	json.Unmarshal([]byte(raw), &meta)

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
	rawAuth := string(bodyAuth)
	if strings.HasPrefix(rawAuth, "&&&START&&&") {
		rawAuth = rawAuth[11:]
	}

	var resAuth struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Location    string `json:"location"`
		Nonce       int64  `json:"nonce"`
		PassToken   string `json:"passToken"`
		Ssecurity   string `json:"ssecurity"`
		UserID      int64  `json:"userId"`
		NotificationURL string `json:"notificationUrl"`
	}
	if err := json.Unmarshal([]byte(rawAuth), &resAuth); err != nil {
		return fmt.Errorf("parse auth response failed: %v", err)
	}

	if resAuth.NotificationURL != "" || resAuth.Code == 70016 || resAuth.Code == 87001 {
		return fmt.Errorf("security_verification_required: 请使用小米账号扫码登录或获取Token")
	}

	if resAuth.Location == "" {
		return fmt.Errorf("login failed (code %d): %s", resAuth.Code, resAuth.Description)
	}

	a.UserID = fmt.Sprintf("%d", resAuth.UserID)
	a.PassToken = resAuth.PassToken
	a.Tokens["micoapi"] = &Token{
		Token:     resAuth.PassToken,
		Ssecurity: resAuth.Ssecurity,
	}

	return a.Save()
}
