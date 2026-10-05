package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"
)

type config struct {
	URL      string `json:"url"`
	Token    string `json:"token"`
	Interval int    `json:"interval"`
	Language string `json:"language"`
	Touch    struct {
		Device   string `json:"device"`
		SwapAxes bool   `json:"swapAxes"`
		MirrorX  bool   `json:"mirrorX"`
		MirrorY  bool   `json:"mirrorY"`
	} `json:"touch"`
	KeyDevices []string `json:"keyDevices"`
}

func loadConfig(path string) (config, error) {
	result := config{Interval: 15, Language: "en", KeyDevices: []string{"/dev/input/event0", "/dev/input/event3"}}
	result.Touch.Device = "/dev/input/event1"
	result.Touch.SwapAxes = true
	result.Touch.MirrorY = true
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	return result, json.Unmarshal(data, &result)
}

func main() {
	home := filepath.Dir(os.Args[0])
	configPath := flag.String("config", filepath.Join(home, "config.json"), "config file")
	fontsDir := flag.String("fonts", filepath.Join(home, "fonts"), "font directory")
	fbink := flag.String("fbink", filepath.Join(home, "fbink"), "fbink binary")
	flag.Parse()
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalln("config:", err)
	}
	regular, err := os.ReadFile(filepath.Join(*fontsDir, "NotoSansJP-Regular.otf"))
	if err != nil {
		log.Fatalln("font:", err)
	}
	bold, err := os.ReadFile(filepath.Join(*fontsDir, "NotoSansJP-Bold.otf"))
	if err != nil {
		log.Fatalln("font:", err)
	}
	loadedFonts, err := loadFonts(loadFontsArgs{regular: regular, bold: bold})
	if err != nil {
		log.Fatalln("font:", err)
	}
	screen, err := openDisplay(*fbink)
	if err != nil {
		log.Fatalln("display:", err)
	}
	defer screen.close()
	gestures := make(chan gesture, 16)
	keys := make(chan keyPress, 16)
	inputErrors := make(chan error, 1)
	mapping := touchMapping{swapAxes: cfg.Touch.SwapAxes, mirrorX: cfg.Touch.MirrorX, mirrorY: cfg.Touch.MirrorY, width: screen.width, height: screen.height}
	go readTouch(readTouchArgs{path: cfg.Touch.Device, mapping: mapping, gestures: gestures, errors: inputErrors})
	for _, device := range cfg.KeyDevices {
		go readKeys(readKeysArgs{path: device, keys: keys})
	}
	instance := &app{
		display:  screen,
		fonts:    loadedFonts,
		relay:    newRelayClient(relayClientArgs{url: cfg.URL, token: cfg.Token}),
		tr:       translator{language: cfg.Language},
		interval: time.Duration(max(cfg.Interval, 5)) * time.Second,
		drafts:   map[string]*draft{},
		exit:     make(chan struct{}),
		results:  make(chan any, 8),
		pollNow:  make(chan struct{}, 1),
	}
	instance.run(runArgs{gestures: gestures, keys: keys, inputErrors: inputErrors})
}
