package dlna

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func parseSOAPAction(header string) string {
	header = strings.Trim(strings.TrimSpace(header), `"`)
	if index := strings.LastIndex(header, "#"); index >= 0 {
		return header[index+1:]
	}
	return header
}

func parseSOAPParams(body []byte) map[string]string {
	params := make(map[string]string)
	decoder := xml.NewDecoder(strings.NewReader(string(body)))
	var stack []string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch value := token.(type) {
		case xml.StartElement:
			stack = append(stack, value.Name.Local)
		case xml.CharData:
			if len(stack) > 0 {
				text := strings.TrimSpace(string(value))
				if text != "" {
					params[stack[len(stack)-1]] += text
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return params
}

func readSOAPRequest(w http.ResponseWriter, r *http.Request) (string, map[string]string, bool) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return "", nil, false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
	if err != nil {
		writeXML(w, http.StatusBadRequest, soapFault(402, "Invalid Args"))
		return "", nil, false
	}
	return parseSOAPAction(r.Header.Get("SOAPAction")), parseSOAPParams(body), true
}

func soapResponse(urn, action string, values map[string]string) string {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:`)
	body.WriteString(action)
	body.WriteString(`Response xmlns:u="`)
	body.WriteString(urn)
	body.WriteString(`">`)
	for key, value := range values {
		body.WriteString("<")
		body.WriteString(key)
		body.WriteString(">")
		body.WriteString(xmlEscape(value))
		body.WriteString("</")
		body.WriteString(key)
		body.WriteString(">")
	}
	body.WriteString("</u:")
	body.WriteString(action)
	body.WriteString("Response></s:Body></s:Envelope>")
	return body.String()
}

func soapFault(code int, description string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>%d</errorCode><errorDescription>%s</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`, code, xmlEscape(description))
}

func (s *Server) handleAVTransport(w http.ResponseWriter, r *http.Request) {
	action, params, ok := readSOAPRequest(w, r)
	if !ok {
		return
	}
	values, code, description := s.runAVTransportAction(action, params, r.RemoteAddr)
	if code != 0 {
		log.Printf("[DLNA] AVTransport action %q from %s returned error %d: %s", action, r.RemoteAddr, code, description)
		writeXML(w, http.StatusInternalServerError, soapFault(code, description))
		return
	}
	writeXML(w, http.StatusOK, soapResponse(avTransportURN, action, values))
}

func (s *Server) runAVTransportAction(action string, params map[string]string, remoteAddr string) (map[string]string, int, string) {
	switch action {
	case "SetAVTransportURI":
		uri := params["CurrentURI"]
		log.Printf("[DLNA] SetAVTransportURI from %s: %s", remoteAddr, uri)
		if err := validateMediaURL(uri); err != nil {
			log.Printf("[DLNA] Invalid media URI from %s: %v", remoteAddr, err)
			return nil, 714, "Illegal MIME-type or media URL"
		}
		s.mu.RLock()
		active := s.activeSession
		s.mu.RUnlock()
		if active != "" {
			s.cancelMedia(active, true)
		}
		s.mu.Lock()
		s.currentURI = uri
		s.currentMetadata = params["CurrentURIMetaData"]
		s.transportState = "STOPPED"
		s.mu.Unlock()
		return map[string]string{}, 0, ""

	case "Play":
		s.mu.RLock()
		uri := s.currentURI
		s.mu.RUnlock()
		log.Printf("[DLNA] Play action from %s for URI: %s", remoteAddr, uri)
		if uri == "" {
			return nil, 701, "Transition not available"
		}
		media := s.newMediaSession(uri)
		streamURL := fmt.Sprintf("http://%s:%d/media/%s", s.HostIP, s.Port, media.id)
		granted := true
		if s.OnSessionStart != nil {
			granted = s.OnSessionStart(SessionInfo{
				ID:        media.id,
				Device:    remoteHost(remoteAddr),
				StreamURL: streamURL,
				Cancel:    func() { s.cancelMedia(media.id, true) },
			})
		}
		s.mu.Lock()
		_, exists := s.media[media.id]
		if granted && exists {
			s.activeSession = media.id
			s.transportState = "PLAYING"
			s.startedAt = time.Now()
		}
		s.mu.Unlock()
		if !granted || !exists {
			s.cancelMedia(media.id, false)
			return nil, 705, "Transport is locked"
		}
		if s.OnSessionActivity != nil {
			s.OnSessionActivity(media.id)
		}
		return map[string]string{}, 0, ""

	case "Pause":
		s.mu.RLock()
		active := s.activeSession
		s.mu.RUnlock()
		if active != "" {
			s.cancelMedia(active, true)
		}
		s.mu.Lock()
		s.transportState = "PAUSED_PLAYBACK"
		s.mu.Unlock()
		return map[string]string{}, 0, ""

	case "Stop":
		s.mu.RLock()
		active := s.activeSession
		s.mu.RUnlock()
		if active != "" {
			s.cancelMedia(active, true)
		}
		s.mu.Lock()
		s.transportState = "STOPPED"
		s.mu.Unlock()
		return map[string]string{}, 0, ""

	case "GetTransportInfo":
		s.mu.RLock()
		state := s.transportState
		s.mu.RUnlock()
		return map[string]string{"CurrentTransportState": state, "CurrentTransportStatus": "OK", "CurrentSpeed": "1"}, 0, ""

	case "GetPositionInfo":
		s.mu.RLock()
		uri, metadata, state, started := s.currentURI, s.currentMetadata, s.transportState, s.startedAt
		s.mu.RUnlock()
		elapsed := time.Duration(0)
		if state == "PLAYING" {
			elapsed = time.Since(started)
		}
		position := durationString(elapsed)
		return map[string]string{"Track": "1", "TrackDuration": "00:00:00", "TrackMetaData": metadata, "TrackURI": uri, "RelTime": position, "AbsTime": position, "RelCount": "0", "AbsCount": "0"}, 0, ""

	case "GetMediaInfo":
		s.mu.RLock()
		uri, metadata := s.currentURI, s.currentMetadata
		s.mu.RUnlock()
		return map[string]string{"NrTracks": "1", "MediaDuration": "00:00:00", "CurrentURI": uri, "CurrentURIMetaData": metadata, "NextURI": "", "NextURIMetaData": "", "PlayMedium": "NETWORK", "RecordMedium": "NOT_IMPLEMENTED", "WriteStatus": "NOT_IMPLEMENTED"}, 0, ""

	case "GetTransportSettings":
		return map[string]string{"PlayMode": "NORMAL", "RecQualityMode": "NOT_IMPLEMENTED"}, 0, ""

	case "GetCurrentTransportActions":
		s.mu.RLock()
		hasURI := s.currentURI != ""
		s.mu.RUnlock()
		actions := ""
		if hasURI {
			actions = "Play,Pause,Stop"
		}
		return map[string]string{"Actions": actions}, 0, ""

	case "Seek":
		return nil, 710, "Seek mode not supported"
	case "Next", "Previous", "SetNextAVTransportURI":
		return nil, 701, "Transition not available"
	default:
		return nil, 401, "Invalid Action"
	}
}

func (s *Server) handleRenderingControl(w http.ResponseWriter, r *http.Request) {
	action, params, ok := readSOAPRequest(w, r)
	if !ok {
		return
	}
	values, code, description := s.runRenderingAction(action, params)
	if code != 0 {
		writeXML(w, http.StatusInternalServerError, soapFault(code, description))
		return
	}
	writeXML(w, http.StatusOK, soapResponse(renderingControlURN, action, values))
}

func (s *Server) runRenderingAction(action string, params map[string]string) (map[string]string, int, string) {
	switch action {
	case "GetVolume":
		volume := 50
		if s.GetVolume != nil {
			volume = s.GetVolume()
		} else {
			s.mu.RLock()
			volume = s.volume
			s.mu.RUnlock()
		}
		return map[string]string{"CurrentVolume": fmt.Sprintf("%d", volume)}, 0, ""
	case "SetVolume":
		volume := parseVolume(params["DesiredVolume"])
		s.mu.Lock()
		s.volume = volume
		active := s.activeSession
		s.mu.Unlock()
		if active != "" && s.OnVolume != nil {
			s.OnVolume(active, volume)
		}
		return map[string]string{}, 0, ""
	case "GetMute":
		s.mu.RLock()
		muted := s.muted
		s.mu.RUnlock()
		value := "0"
		if muted {
			value = "1"
		}
		return map[string]string{"CurrentMute": value}, 0, ""
	case "SetMute":
		muted := params["DesiredMute"] == "1" || strings.EqualFold(params["DesiredMute"], "true")
		s.mu.Lock()
		s.muted = muted
		active, volume := s.activeSession, s.volume
		s.mu.Unlock()
		if active != "" && s.OnVolume != nil {
			if muted {
				s.OnVolume(active, 0)
			} else {
				s.OnVolume(active, volume)
			}
		}
		return map[string]string{}, 0, ""
	default:
		return nil, 401, "Invalid Action"
	}
}

func (s *Server) handleConnectionManager(w http.ResponseWriter, r *http.Request) {
	action, _, ok := readSOAPRequest(w, r)
	if !ok {
		return
	}
	var values map[string]string
	switch action {
	case "GetProtocolInfo":
		values = map[string]string{
			"Source": "",
			"Sink": strings.Join([]string{
				"http-get:*:audio/mpeg:*",
				"http-get:*:audio/mp4:*",
				"http-get:*:audio/aac:*",
				"http-get:*:audio/flac:*",
				"http-get:*:audio/wav:*",
				"http-get:*:audio/x-wav:*",
				"http-get:*:audio/ogg:*",
			}, ","),
		}
	case "GetCurrentConnectionIDs":
		values = map[string]string{"ConnectionIDs": "0"}
	case "GetCurrentConnectionInfo":
		values = map[string]string{"RcsID": "0", "AVTransportID": "0", "ProtocolInfo": "", "PeerConnectionManager": "", "PeerConnectionID": "-1", "Direction": "Input", "Status": "OK"}
	default:
		writeXML(w, http.StatusInternalServerError, soapFault(401, "Invalid Action"))
		return
	}
	writeXML(w, http.StatusOK, soapResponse(connectionManagerURN, action, values))
}
