package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func externalPowerConnected() bool {
	supplies, _ := filepath.Glob("/sys/class/power_supply/*")
	for _, supply := range supplies {
		kind, _ := os.ReadFile(filepath.Join(supply, "type"))
		if strings.TrimSpace(string(kind)) == "Battery" {
			continue
		}
		online, _ := os.ReadFile(filepath.Join(supply, "online"))
		if value := strings.TrimSpace(string(online)); value != "" && value != "0" {
			return true
		}
	}
	return false
}

type cableConnected struct{}

func watchCable(args watchCableArgs) {
	connected := externalPowerConnected()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := externalPowerConnected()
			if now && !connected {
				args.events <- cableConnected{}
			}
			connected = now
		case <-args.exit:
			return
		}
	}
}

type watchCableArgs struct {
	events chan<- any
	exit   <-chan struct{}
}
