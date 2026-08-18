package miservice

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccount_EnsureAndRefreshToken(t *testing.T) {
	var serviceLoginCalls atomic.Int32
	var stsCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pass/serviceLogin":
			serviceLoginCalls.Add(1)
			data := map[string]interface{}{
				"code":      0,
				"location":  "http://" + r.Host + "/sts?d=test",
				"nonce":     123456789,
				"ssecurity": "c2VjdXJpdHl0ZXN0MTIzNA==",
				"desc":      "成功",
			}
			raw, _ := json.Marshal(data)
			w.Write([]byte("&&&START&&&" + string(raw)))
		case r.URL.Path == "/sts":
			stsCalls.Add(1)
			http.SetCookie(w, &http.Cookie{
				Name:  "serviceToken",
				Value: "mock_service_token_value",
			})
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "token.json")

	acc := NewAccount(storeFile)
	acc.Data.UserID = "123456"
	acc.Data.PassToken = "mock_pass_token"

	// Override httpClient transport for testing
	oldTransport := acc.httpClient.Transport
	acc.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		newURL, _ := url.Parse(server.URL + req.URL.Path + "?" + req.URL.RawQuery)
		req.URL = newURL
		return http.DefaultTransport.RoundTrip(req)
	})
	defer func() { acc.httpClient.Transport = oldTransport }()

	tok, err := acc.EnsureToken("micoapi")
	if err != nil {
		t.Fatalf("EnsureToken failed: %v", err)
	}
	if tok.Token != "mock_service_token_value" {
		t.Errorf("expected token mock_service_token_value, got %s", tok.Token)
	}

	// Verify it was saved to file
	savedData, err := os.ReadFile(storeFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var stored StoreData
	if err := json.Unmarshal(savedData, &stored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if stored.Tokens["micoapi"].Token != "mock_service_token_value" {
		t.Errorf("expected stored token to be saved")
	}

	// Calling EnsureToken again should use cached token without extra calls
	tok2, err := acc.EnsureToken("micoapi")
	if err != nil {
		t.Fatalf("second EnsureToken failed: %v", err)
	}
	if tok2.Token != "mock_service_token_value" {
		t.Errorf("expected cached token")
	}
	if serviceLoginCalls.Load() != 1 {
		t.Errorf("expected 1 serviceLogin call, got %d", serviceLoginCalls.Load())
	}
}

func TestAccount_RequestMina_401AutoRetry(t *testing.T) {
	var minaCalls atomic.Int32
	var loginCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pass/serviceLogin":
			loginCalls.Add(1)
			data := map[string]interface{}{
				"code":      0,
				"location":  "http://" + r.Host + "/sts",
				"nonce":     987654321,
				"ssecurity": "c2VjdXJpdHl0ZXN0MTIzNA==",
			}
			raw, _ := json.Marshal(data)
			w.Write([]byte(string(raw)))
		case r.URL.Path == "/sts":
			http.SetCookie(w, &http.Cookie{
				Name:  "serviceToken",
				Value: "fresh_token_123",
			})
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/remote/ubus":
			callNum := minaCalls.Add(1)
			if callNum == 1 {
				// First call simulates expired token
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("<html>401 Unauthorized</html>"))
				return
			}
			// Second call after refresh
			cookie, _ := r.Cookie("serviceToken")
			if cookie == nil || cookie.Value != "fresh_token_123" {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("invalid cookie"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"code":0,"message":"Success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "token.json")

	acc := NewAccount(storeFile)
	acc.Data.UserID = "123456"
	acc.Data.PassToken = "mock_pass_token"
	acc.Data.Tokens["micoapi"] = TokenData{Token: "expired_token"}

	acc.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		newURL, _ := url.Parse(server.URL + req.URL.Path + "?" + req.URL.RawQuery)
		req.URL = newURL
		return http.DefaultTransport.RoundTrip(req)
	})

	err := acc.PlayByMusicURL("dummy_did", "http://192.168.10.1:8300/stream/a1.wav")
	if err != nil {
		t.Fatalf("PlayByMusicURL failed: %v", err)
	}

	if minaCalls.Load() != 2 {
		t.Errorf("expected 2 mina calls (1 fail + 1 retry), got %d", minaCalls.Load())
	}
	if loginCalls.Load() != 1 {
		t.Errorf("expected 1 token refresh call, got %d", loginCalls.Load())
	}
}

func TestAccount_DeviceListAndOperations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admin/v2/device_list":
			w.Write([]byte(`{"code":0,"message":"Success","data":[{"deviceID":"dev1","name":"Speaker 1","hardware":"LX04"}]}`))
		case "/remote/ubus":
			w.Write([]byte(`{"code":0,"message":"Success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "token.json")

	acc := NewAccount(storeFile)
	acc.Data.UserID = "123456"
	acc.Data.PassToken = "mock_pass"
	acc.Data.Tokens["micoapi"] = TokenData{Token: "valid_token"}

	acc.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		newURL, _ := url.Parse(server.URL + req.URL.Path + "?" + req.URL.RawQuery)
		req.URL = newURL
		return http.DefaultTransport.RoundTrip(req)
	})

	devs, err := acc.DeviceList(0)
	if err != nil {
		t.Fatalf("DeviceList failed: %v", err)
	}
	if len(devs) != 1 || devs[0].DeviceID != "dev1" {
		t.Errorf("unexpected devices: %+v", devs)
	}

	if err := acc.PlayerPause("dev1"); err != nil {
		t.Errorf("PlayerPause failed: %v", err)
	}
	if err := acc.PlayerSetVolume("dev1", 80); err != nil {
		t.Errorf("PlayerSetVolume failed: %v", err)
	}
	if err := acc.PlayerStop("dev1"); err != nil {
		t.Errorf("PlayerStop failed: %v", err)
	}
}

func TestAccount_MinaErrorReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":-1,"message":"Internal Error"}`))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "token.json")

	acc := NewAccount(storeFile)
	acc.Data.UserID = "123456"
	acc.Data.PassToken = "mock_pass"
	acc.Data.Tokens["micoapi"] = TokenData{Token: "valid_token"}

	acc.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		newURL, _ := url.Parse(server.URL + req.URL.Path + "?" + req.URL.RawQuery)
		req.URL = newURL
		return http.DefaultTransport.RoundTrip(req)
	})

	err := acc.PlayByMusicURL("dev1", "http://test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAccount_AutoRefresh(t *testing.T) {
	var refreshCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/pass/serviceLogin":
			data := map[string]interface{}{
				"code":      0,
				"location":  "http://" + r.Host + "/sts?d=test",
				"nonce":     123456789,
				"ssecurity": "c2VjdXJpdHl0ZXN0MTIzNA==",
			}
			raw, _ := json.Marshal(data)
			w.Write([]byte("&&&START&&&" + string(raw)))
		case r.URL.Path == "/sts":
			refreshCount.Add(1)
			http.SetCookie(w, &http.Cookie{
				Name:  "serviceToken",
				Value: "auto_token",
			})
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "token.json")

	acc := NewAccount(storeFile)
	acc.Data.UserID = "123456"
	acc.Data.PassToken = "mock_pass"

	acc.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		newURL, _ := url.Parse(server.URL + req.URL.Path + "?" + req.URL.RawQuery)
		req.URL = newURL
		return http.DefaultTransport.RoundTrip(req)
	})

	// Start auto-refresh with very short interval for test
	acc.StartAutoRefresh(15 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	acc.StopAutoRefresh()

	st := acc.GetTokenStatus()
	if !st.Valid || !st.HasCredentials {
		t.Errorf("expected valid token status, got %+v", st)
	}
	if refreshCount.Load() < 2 {
		t.Errorf("expected at least 2 refreshes (initial + periodic), got %d", refreshCount.Load())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
