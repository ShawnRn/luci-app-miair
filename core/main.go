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
	flagCookie = flag.String("cookie", "", "Mi Account Cookie file or string (optional)")
	flagStore  = flag.String("store", "/tmp/.mi_token.json", "Token store path")
	flagList   = flag.Bool("list", false, "List XiaoAi devices in account")
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

	if *flagList {
		if *flagUser == "" || *flagPass == "" {
			log.Fatal("Username and password required for listing devices")
		}
		client := miservice.NewClient(*flagUser, *flagPass, *flagCookie, *flagStore)
		mina := miservice.NewMiNAService(client)
		devs, err := mina.DeviceList(0)
		if err != nil {
			log.Fatalf("Failed to get devices: %v", err)
		}
		fmt.Printf("Found %d devices:\n", len(devs))
		for _, d := range devs {
			fmt.Printf("DID: %s | Name: %s | Hardware: %s | IP: %s\n", d.DeviceID, d.Name, d.Hardware, d.CurrentLocalIP)
		}
		return
	}

	if *flagDevice == "" {
		log.Println("No --device specified. AirPlay server will stream audio locally without pushing to XiaoAi.")
	}

	var mina *miservice.MiNAService
	if *flagUser != "" && *flagPass != "" {
		client := miservice.NewClient(*flagUser, *flagPass, *flagCookie, *flagStore)
		mina = miservice.NewMiNAService(client)
	}

	localIP := getLocalIP()
	streamURL := fmt.Sprintf("http://%s:%d/stream.wav", localIP, *flagHTTP)

	server, err := airplay.NewServer(*flagName, *flagPort, *flagHTTP, "/stream.wav")
	if err != nil {
		log.Fatalf("Failed to create AirPlay server: %v", err)
	}

	server.OnPlay = func() {
		log.Printf("AirPlay streaming started! Stream URL: %s", streamURL)
		if mina != nil && *flagDevice != "" {
			go func() {
				// Retry or call play_by_music_url
				log.Printf("Instructing XiaoAi [%s] to play stream...", *flagDevice)
				err := mina.PlayByMusicURL(*flagDevice, streamURL)
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
		if mina != nil && *flagDevice != "" {
			go func() {
				log.Printf("Stopping XiaoAi [%s]...", *flagDevice)
				_ = mina.PlayerPause(*flagDevice)
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
