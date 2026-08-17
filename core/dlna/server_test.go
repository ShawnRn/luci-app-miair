package dlna

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeviceAndServiceDescriptionsAreValidXML(t *testing.T) {
	documents := []string{
		deviceDescription(`Living Room & "Speaker"`, "uuid:test"),
		avTransportSCPD,
		renderingControlSCPD,
		connectionManagerSCPD,
		soapResponse(avTransportURN, "Play", map[string]string{}),
		soapFault(705, "Transport is locked"),
	}
	for index, document := range documents {
		var value any
		if err := xml.Unmarshal([]byte(document), &value); err != nil {
			t.Fatalf("document %d is invalid XML: %v", index, err)
		}
	}
}

func TestSOAPParameterParsing(t *testing.T) {
	body := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:SetAVTransportURI xmlns:u="urn:test"><InstanceID>0</InstanceID><CurrentURI>http://phone/music.mp3</CurrentURI><CurrentURIMetaData>&lt;title&gt;Song&lt;/title&gt;</CurrentURIMetaData></u:SetAVTransportURI></s:Body></s:Envelope>`
	params := parseSOAPParams([]byte(body))
	if params["CurrentURI"] != "http://phone/music.mp3" {
		t.Fatalf("CurrentURI = %q", params["CurrentURI"])
	}
	if params["CurrentURIMetaData"] != "<title>Song</title>" {
		t.Fatalf("metadata = %q", params["CurrentURIMetaData"])
	}
}

func TestDLNAPlayCreatesUniqueProxySession(t *testing.T) {
	server := NewServer("test", "192.168.10.1", 8301)
	var started SessionInfo
	server.OnSessionStart = func(info SessionInfo) bool {
		started = info
		return true
	}

	if _, code, _ := server.runAVTransportAction("SetAVTransportURI", map[string]string{"CurrentURI": "http://phone/music.mp3"}, "192.168.10.22:1234"); code != 0 {
		t.Fatalf("SetAVTransportURI failed with code %d", code)
	}
	if _, code, description := server.runAVTransportAction("Play", map[string]string{}, "192.168.10.22:1234"); code != 0 {
		t.Fatalf("Play failed with code %d: %s", code, description)
	}
	if started.ID == "" || started.Device != "192.168.10.22" {
		t.Fatalf("started session = %+v", started)
	}
	if !strings.HasSuffix(started.StreamURL, "/media/"+started.ID) {
		t.Fatalf("stream URL = %q", started.StreamURL)
	}
	server.mu.RLock()
	state, active := server.transportState, server.activeSession
	server.mu.RUnlock()
	if state != "PLAYING" || active != started.ID {
		t.Fatalf("state=%s active=%s", state, active)
	}
}

func TestDLNAPlayReturnsTransportLockedWhenRejected(t *testing.T) {
	server := NewServer("test", "192.168.10.1", 8301)
	server.OnSessionStart = func(SessionInfo) bool { return false }
	server.currentURI = "http://phone/music.mp3"
	if _, code, _ := server.runAVTransportAction("Play", map[string]string{}, "192.168.10.22:1234"); code != 705 {
		t.Fatalf("Play error code = %d, want 705", code)
	}
	if len(server.media) != 0 {
		t.Fatalf("rejected session leaked %d media entries", len(server.media))
	}
}

func TestMediaProxyForwardsRangeRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=2-5" {
			t.Errorf("Range = %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Range", "bytes 2-5/8")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "2345")
	}))
	defer upstream.Close()

	server := NewServer("test", "127.0.0.1", 8301)
	media := server.newMediaSession(upstream.URL + "/music.mp3")
	request := httptest.NewRequest(http.MethodGet, "/media/"+media.id, nil)
	request.Header.Set("Range", "bytes=2-5")
	recorder := httptest.NewRecorder()
	server.handleMediaProxy(recorder, request)
	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "2345" {
		t.Fatalf("proxy response = %d %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Range") != "bytes 2-5/8" {
		t.Fatalf("Content-Range = %q", recorder.Header().Get("Content-Range"))
	}
}
