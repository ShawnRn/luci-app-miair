package miservice

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
)

type DeviceInfo struct {
	DeviceID       string `json:"deviceID"`
	SerialNumber   string `json:"serialNumber"`
	Name           string `json:"name"`
	Alias          string `json:"alias"`
	Hardware       string `json:"hardware"`
	Mac            string `json:"mac"`
	CurrentLocalIP string `json:"currentLocalIP"`
}

type DeviceListResponse struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    []DeviceInfo `json:"data"`
}

type MiNAService struct {
	client *Client
}

func NewMiNAService(client *Client) *MiNAService {
	return &MiNAService{client: client}
}

func (s *MiNAService) DeviceList(master int) ([]DeviceInfo, error) {
	params := url.Values{}
	params.Set("master", strconv.Itoa(master))
	resp, err := s.client.GetMina("/admin/v2/device_list", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res DeviceListResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	if res.Code != 0 {
		return nil, fmt.Errorf("device_list error code: %d, msg: %s", res.Code, res.Message)
	}

	return res.Data, nil
}

func (s *MiNAService) PlayByMusicURL(deviceID string, streamURL string) error {
	data := url.Values{}
	data.Set("deviceId", deviceID)
	data.Set("url", streamURL)
	data.Set("type", "1")

	resp, err := s.client.PostMina("/music/play_by_music_url", data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &res); err == nil && res.Code != 0 {
		return fmt.Errorf("play_by_music_url failed: code=%d msg=%s", res.Code, res.Message)
	}
	return nil
}

func (s *MiNAService) PlayerPause(deviceID string) error {
	data := url.Values{}
	data.Set("deviceId", deviceID)
	data.Set("type", "1")
	resp, err := s.client.PostMina("/player/pause", data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (s *MiNAService) PlayerStop(deviceID string) error {
	data := url.Values{}
	data.Set("deviceId", deviceID)
	data.Set("type", "1")
	resp, err := s.client.PostMina("/player/stop", data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
