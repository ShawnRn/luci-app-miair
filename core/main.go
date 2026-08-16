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

var (
	flagDevice = flag.String("device", "", "Device ID (did) of XiaoAi speaker")
	flagName   = flag.String("name", "XiaoAi AirPlay", "AirPlay device name")
	flagPort   = flag.Int("port", 5000, "AirPlay RTSP port")
	flagHTTP   = flag.Int("http-port", 8300, "Local HTTP audio stream port")
	flagUser   = flag.String("user", "", "Mi Account Username")
	flagPass   = flag.String("pass", "", "Mi Account Password")
	flagStore  = flag.String("store", "/etc/miair/token.json", "Token store path")
	flagList   = flag.Bool("list", false, "List XiaoAi devices in account")
	flagQR     = flag.Bool("qr", false, "Start QR login flow")
	flagPollQR = flag.String("poll-qr", "", "Poll QR login lp url")
)

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func main() {
	flag.Parse()

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
			_ = account.LoginByPassword(*flagUser, *flagPass)
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
		_ = account.LoginByPassword(*flagUser, *flagPass)
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
				err := account.PlayByMusicURL(targetDID, streamURL)
				if err != nil {
					log.Printf("Error triggering XiaoAi playback: %v", err)
				} else {
					log.Println("XiaoAi playback triggered successfully!")
				}
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

	log.Printf("=== miair-core started: %s (AirPlay port %d, HTTP stream port %d) ===", *flagName, *flagPort, *flagHTTP)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down miair-core...")
	time.Sleep(500 * time.Millisecond)
}

