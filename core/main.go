package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"miair-core/airplay"
	"miair-core/miservice"
)

var version = "development"

var (
	flagDevice  = flag.String("device", "", "Device ID (did) of XiaoAi speaker")
	flagName    = flag.String("name", "XiaoAi AirPlay", "AirPlay device name")
	flagPort    = flag.Int("port", 5000, "AirPlay RTSP port")
	flagHTTP    = flag.Int("http-port", 8300, "Local HTTP audio stream port")
	flagUser    = flag.String("user", "", "Mi Account Username")
	flagPass    = flag.String("pass", "", "Mi Account Password")
	flagStore   = flag.String("store", "/etc/miair/token.json", "Token store path")
	flagList    = flag.Bool("list", false, "List XiaoAi devices in account")
	flagQR      = flag.Bool("qr", false, "Start QR login flow")
	flagPollQR  = flag.String("poll-qr", "", "Poll QR login lp url")
	flagVersion = flag.Bool("version", false, "Print version and exit")
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
		if *flagUser != "" && *flagPass != "" && account.Data.PassToken == "" {
		}

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
	if *flagUser != "" && *flagPass != "" && account.Data.PassToken == "" {
	}

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

	localIP := getLocalIP()
	streamURL := fmt.Sprintf("http://%s:%d/stream.wav", localIP, *flagHTTP)

	server, err := airplay.NewServer(*flagName, *flagPort, *flagHTTP, "/stream.wav")
	if err != nil {
		log.Fatalf("Failed to create AirPlay server: %v", err)
	}

	server.OnPlay = func() {
		log.Printf("AirPlay streaming started! Stream URL: %s", streamURL)
		if targetDID != "" {
			go func() {
				log.Printf("Instructing XiaoAi [%s] to play stream...", targetDID)
				// Retry up to 3 times with increasing delay
				for attempt := 1; attempt <= 3; attempt++ {
					err := account.PlayByMusicURL(targetDID, streamURL)
					if err != nil {
						log.Printf("Attempt %d: XiaoAi playback error: %v", attempt, err)
						if attempt < 3 {
							time.Sleep(time.Duration(attempt) * time.Second)
						}
						continue
					}
					log.Printf("XiaoAi playback triggered successfully on attempt %d!", attempt)
					return
				}
				log.Println("All playback attempts failed!")
			}()
		}
	}

	server.OnStop = func() {
		log.Println("AirPlay streaming stopped.")
		if targetDID != "" {
			go func() {
				log.Printf("Stopping XiaoAi [%s]...", targetDID)
				_ = account.PlayerPause(targetDID)
			}()
		}
	}

	err = server.Start()
	if err != nil {
		log.Fatalf("Failed to start AirPlay server: %v", err)
	}

	log.Printf("=== miair-core %s started: %s (AirPlay port %d, HTTP stream port %d) ===", version, *flagName, *flagPort, *flagHTTP)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down miair-core...")
	time.Sleep(500 * time.Millisecond)
}
