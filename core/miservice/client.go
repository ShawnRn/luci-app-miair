package miservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TokenStore struct {
	UserID       string `json:"userId"`
	PassToken    string `json:"passToken"`
	SSecurity    string `json:"ssecurity"`
	ServiceToken string `json:"serviceToken"`
	DeviceID     string `json:"deviceId"`
}

type Client struct {
	Account  string
	Password string
	Cookie   string
	Token    *TokenStore

	httpClient *http.Client
	storePath  string
}

func NewClient(account, password, cookie, storePath string) *Client {
	jar, _ := cookiejar.New(nil)
	client := &Client{
		Account:    account,
		Password:   password,
		Cookie:     cookie,
		Token:      &TokenStore{DeviceID: "miair_" + RandomHex(8)},
		httpClient: &http.Client{Jar: jar, Timeout: 15 * time.Second},
		storePath:  storePath,
	}
	client.LoadToken()
	return client
}

func (c *Client) LoadToken() {
	if c.storePath == "" {
		return
	}
	data, err := os.ReadFile(c.storePath)
	if err == nil {
		json.Unmarshal(data, c.Token)
	}
}

func (c *Client) SaveToken() {
	if c.storePath == "" {
		return
	}
	os.MkdirAll(filepath.Dir(c.storePath), 0755)
	data, _ := json.MarshalIndent(c.Token, "", "  ")
	os.WriteFile(c.storePath, data, 0600)
}

func (c *Client) ParseCookie() bool {
	if c.Cookie == "" {
		return false
	}
	for _, item := range strings.Split(c.Cookie, ";") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) == 2 {
			k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if k == "userId" {
				c.Token.UserID = v
			} else if k == "passToken" {
				c.Token.PassToken = v
			}
		}
	}
	return c.Token.UserID != "" && c.Token.PassToken != ""
}

func (c *Client) Login(sid string) error {
	c.ParseCookie()
	if c.Token.UserID != "" && c.Token.PassToken != "" {
		return c.loginWithPassToken(sid)
	}
	if c.Account == "" || c.Password == "" {
		return errors.New("empty credentials and no passToken")
	}
	return c.loginWithPassword(sid)
}

func (c *Client) loginWithPassword(sid string) error {
	signURL := "https://account.xiaomi.com/pass/serviceLogin?sid=" + sid + "&_json=true"
	resp, err := c.httpClient.Get(signURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	raw := strings.TrimPrefix(string(body), "&&&START&&&")
	var signResp struct {
		QS   string `json:"qs"`
		Sign string `json:"_sign"`
		SID  string `json:"sid"`
		Code int    `json:"code"`
		Desc string `json:"description"`
	}
	if err := json.Unmarshal([]byte(raw), &signResp); err != nil {
		return err
	}

	authURL := "https://account.xiaomi.com/pass/serviceLoginAuth2"
	data := url.Values{}
	data.Set("_json", "true")
	data.Set("qs", signResp.QS)
	data.Set("sid", signResp.SID)
	data.Set("_sign", signResp.Sign)
	data.Set("user", c.Account)
	data.Set("hash", strings.ToUpper(MD5Hex([]byte(c.Password))))

	req, _ := http.NewRequest("POST", authURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "APP/com.xiaomi.mihome APPV/6.0.103 iosPassportSDK/3.9.0 iOS/14.4 miHSTS")

	respAuth, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer respAuth.Body.Close()

	bodyAuth, _ := io.ReadAll(respAuth.Body)
	rawAuth := strings.TrimPrefix(string(bodyAuth), "&&&START&&&")

	var authResult struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Location    string `json:"location"`
		Nonce       int64  `json:"nonce"`
		PassToken   string `json:"passToken"`
		SSecurity   string `json:"ssecurity"`
		UserID      int64  `json:"userId"`
	}
	if err := json.Unmarshal([]byte(rawAuth), &authResult); err != nil {
		return err
	}

	if authResult.Code != 0 {
		return fmt.Errorf("login failed: %s (code %d)", authResult.Description, authResult.Code)
	}

	c.Token.UserID = fmt.Sprintf("%d", authResult.UserID)
	c.Token.PassToken = authResult.PassToken
	c.Token.SSecurity = authResult.SSecurity

	return c.loginWithLocation(authResult.Location)
}

func (c *Client) loginWithPassToken(sid string) error {
	serviceLoginURL := fmt.Sprintf("https://account.xiaomi.com/pass/serviceLogin?sid=%s&_json=true", sid)
	req, _ := http.NewRequest("GET", serviceLoginURL, nil)
	req.Header.Set("Cookie", fmt.Sprintf("userId=%s; passToken=%s", c.Token.UserID, c.Token.PassToken))
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	raw := strings.TrimPrefix(string(body), "&&&START&&&")

	var res struct {
		Code      int    `json:"code"`
		Location  string `json:"location"`
		Nonce     int64  `json:"nonce"`
		SSecurity string `json:"ssecurity"`
		UserID    int64  `json:"userId"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return err
	}

	if res.Location == "" {
		return fmt.Errorf("login with passToken failed: code=%d", res.Code)
	}

	c.Token.SSecurity = res.SSecurity
	return c.loginWithLocation(res.Location)
}

func (c *Client) loginWithLocation(location string) error {
	resp, err := c.httpClient.Get(location)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "serviceToken" {
			c.Token.ServiceToken = cookie.Value
			break
		}
	}

	if c.Token.ServiceToken == "" {
		return errors.New("serviceToken not found in login response")
	}

	c.SaveToken()
	return nil
}

func (c *Client) GetMina(path string, params url.Values) (*http.Response, error) {
	if c.Token.ServiceToken == "" || c.Token.SSecurity == "" {
		if err := c.Login("micoapi"); err != nil {
			return nil, err
		}
	}

	minaURL := "https://api2.mina.mi.com" + path
	if len(params) > 0 {
		minaURL += "?" + params.Encode()
	}

	req, _ := http.NewRequest("GET", minaURL, nil)
	req.Header.Set("User-Agent", "MISoundBox/1.4.0, iOS/14.4")
	req.Header.Set("Cookie", fmt.Sprintf("userId=%s; serviceToken=%s", c.Token.UserID, c.Token.ServiceToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		if err := c.Login("micoapi"); err == nil {
			req.Header.Set("Cookie", fmt.Sprintf("userId=%s; serviceToken=%s", c.Token.UserID, c.Token.ServiceToken))
			return c.httpClient.Do(req)
		}
	}

	return resp, nil
}

func (c *Client) PostMina(path string, data url.Values) (*http.Response, error) {
	if c.Token.ServiceToken == "" || c.Token.SSecurity == "" {
		if err := c.Login("micoapi"); err != nil {
			return nil, err
		}
	}

	minaURL := "https://api2.mina.mi.com" + path
	req, _ := http.NewRequest("POST", minaURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "MISoundBox/1.4.0, iOS/14.4")
	req.Header.Set("Cookie", fmt.Sprintf("userId=%s; serviceToken=%s", c.Token.UserID, c.Token.ServiceToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		if err := c.Login("micoapi"); err == nil {
			req.Header.Set("Cookie", fmt.Sprintf("userId=%s; serviceToken=%s", c.Token.UserID, c.Token.ServiceToken))
			return c.httpClient.Do(req)
		}
	}

	return resp, nil
}
