package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"miair-core/airplay"
	"miair-core/dlna"
	"miair-core/miservice"
	"miair-core/playback"
	"miair-core/source"
)

var version = "development"

var (
	flagDevice    = flag.String("device", "", "Device ID (did) of XiaoAi speaker")
	flagName      = flag.String("name", "XiaoAi AirPlay", "AirPlay device name")
	flagAirPlay   = flag.Bool("airplay", true, "Enable the AirPlay receiver")
	flagPort      = flag.Int("port", 5000, "AirPlay RTSP port")
	flagHTTP      = flag.Int("http-port", 8300, "Local HTTP audio stream port")
	flagBuffer    = flag.Int("buffer-ms", 1500, "Audio pre-buffer duration in milliseconds (0-5000)")
	flagDLNA      = flag.Bool("dlna", true, "Enable the DLNA/UPnP MediaRenderer")
	flagDLNAPort  = flag.Int("dlna-port", 8301, "DLNA HTTP control and media proxy port")
	flagPolicy    = flag.String("source-policy", "latest", "Source policy: latest, lock, idle, or priority")
	flagIdle      = flag.Int("idle-timeout", 10, "Seconds before an inactive source may be preempted")
	flagPreferred = flag.String("preferred-protocol", "airplay", "Preferred protocol for priority policy: airplay or dlna")
	flagStatus    = flag.String("status-file", "/var/run/miair-status.json", "Runtime status JSON path")
	flagStore     = flag.String("store", "/etc/miair/token.json", "Token store path")
	flagList      = flag.Bool("list", false, "List XiaoAi devices in account")
	flagQR        = flag.Bool("qr", false, "Start QR login flow")
	flagPollQR    = flag.String("poll-qr", "", "Poll QR login lp url")
	flagVersion   = flag.Bool("version", false, "Print version and exit")
)

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	// Prefer LAN bridge IP (192.168.x.x or 10.x.x.x) over WAN
	var fallback string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				ipStr := ip4.String()
				if ip4[0] == 192 && ip4[1] == 168 {
					return ipStr
				}
				if ip4[0] == 10 {
					return ipStr
				}
				if fallback == "" {
					fallback = ipStr
				}
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	return "192.168.10.1"
}

func main() {
	flag.Parse()
	if *flagVersion {
		fmt.Printf("miair-core %s\n", version)
		return
	}
	if *flagBuffer < 0 || *flagBuffer > 5000 {
		log.Fatalf("buffer-ms must be between 0 and 5000")
	}
	if !*flagAirPlay && !*flagDLNA {
		log.Fatalf("at least one receiver protocol must be enabled")
	}
	if *flagIdle < 1 || *flagIdle > 3600 {
		log.Fatalf("idle-timeout must be between 1 and 3600 seconds")
	}

	// 1. QR Code initialization
	if *flagQR {
		account := miservice.NewAccount(*flagStore)
		qrInfo, err := account.GetQRLoginInfo()
		if err != nil {
			log.Fatalf("Failed to get QR info: %v", err)
		}
		fmt.Printf("QR_URL:%s|LP_URL:%s|TIMEOUT:%d\n", qrInfo.QR, qrInfo.LP, qrInfo.Timeout)
		os.Exit(0)
	}

	// 2. Poll QR Code login status
	if *flagPollQR != "" {
		account := miservice.NewAccount(*flagStore)
		status, token, err := account.PollQRLogin(*flagPollQR)
		if err != nil {
			log.Fatalf("Poll error: %v", err)
		}
		if token != nil {
			fmt.Printf("SUCCESS:USER_ID:%s\n", token.UserID)
		} else {
			fmt.Printf("STATUS:%s\n", status)
		}
		os.Exit(0)
	}

	// 3. List XiaoAi devices
	if *flagList {
		account := miservice.NewAccount(*flagStore)
		devs, err := account.DeviceList(0)
		if err != nil {
			log.Fatalf("Failed to get devices: %v", err)
		}
		fmt.Printf("Found %d devices:\n", len(devs))
		for _, d := range devs {
			fmt.Printf("DID: %s | Name: %s | Hardware: %s | IP: %s\n", d.DeviceID, d.Name, d.Hardware, d.CurrentLocalIP)
		}
		return
	}

	// 4. Runtime Mode
	account := miservice.NewAccount(*flagStore)

	targetDID := *flagDevice
	if targetDID == "" {
		// Auto-detect first XiaoAi speaker if not specified
		devs, err := account.DeviceList(0)
		if err == nil && len(devs) > 0 {
			targetDID = devs[0].DeviceID
			log.Printf("Auto-selected XiaoAi speaker: %s (%s)", devs[0].Name, targetDID)
		} else {
			log.Println("No device specified and unable to auto-discover. Stream available at local HTTP port.")
		}
	}

	policy, err := source.ParsePolicy(*flagPolicy)
	if err != nil {
		log.Fatal(err)
	}
	preferred := source.Protocol(strings.ToLower(strings.TrimSpace(*flagPreferred)))
	if preferred != source.ProtocolAirPlay && preferred != source.ProtocolDLNA {
		log.Fatalf("preferred-protocol must be airplay or dlna")
	}
	manager := source.NewManager(policy, time.Duration(*flagIdle)*time.Second, preferred)
	coordinator := playback.NewCoordinator(manager, account, targetDID, *flagStatus)
	defer coordinator.Close()

	localIP := getLocalIP()
	var airplayServer *airplay.Server
	if *flagAirPlay {
		airplayServer, err = airplay.NewServer(*flagName, *flagPort, *flagHTTP, "/stream.wav", *flagBuffer)
		if err != nil {
			log.Fatalf("Failed to create AirPlay server: %v", err)
		}
		airplayServer.OnSessionStart = func(info airplay.SessionInfo) bool {
			streamURL := fmt.Sprintf("http://%s:%d%s", localIP, *flagHTTP, info.StreamPath)
			decision := coordinator.Activate(source.Request{
				ID:        info.ID,
				Protocol:  source.ProtocolAirPlay,
				Device:    info.Device,
				StreamURL: streamURL,
				Cancel:    info.Cancel,
			})
			return decision.Granted
		}
		airplayServer.OnSessionStop = func(sessionID string) { coordinator.Deactivate(sessionID) }
		airplayServer.OnSessionActivity = func(sessionID string) { coordinator.Touch(sessionID) }
		airplayServer.OnVolume = func(sessionID string, volume int) { coordinator.SetVolume(sessionID, volume) }
		if err = airplayServer.Start(); err != nil {
			log.Fatalf("Failed to start AirPlay server: %v", err)
		}
		defer airplayServer.Close()
	}

	var dlnaServer *dlna.Server
	if *flagDLNA {
		dlnaServer = dlna.NewServer(*flagName, localIP, *flagDLNAPort)
		dlnaServer.OnSessionStart = func(info dlna.SessionInfo) bool {
			decision := coordinator.Activate(source.Request{
				ID:        info.ID,
				Protocol:  source.ProtocolDLNA,
				Device:    info.Device,
				StreamURL: info.StreamURL,
				Cancel:    info.Cancel,
			})
			return decision.Granted
		}
		dlnaServer.OnSessionStop = func(sessionID string) { coordinator.Deactivate(sessionID) }
		dlnaServer.OnSessionActivity = func(sessionID string) { coordinator.Touch(sessionID) }
		dlnaServer.OnVolume = func(sessionID string, volume int) { coordinator.SetVolume(sessionID, volume) }
		if err = dlnaServer.Start(); err != nil {
			log.Fatalf("Failed to start DLNA server: %v", err)
		}
		defer dlnaServer.Close()
	}

	log.Printf("=== miair-core %s started: %s (AirPlay=%t, DLNA=%t, source policy=%s) ===", version, *flagName, *flagAirPlay, *flagDLNA, policy)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down miair-core...")
	time.Sleep(500 * time.Millisecond)
}
