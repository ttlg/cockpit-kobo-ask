package main

import (
	"encoding/binary"
	"log"
	"os"
	"syscall"
	"unsafe"
)

const (
	evSyn         = 0
	evKey         = 1
	evAbs         = 3
	absMtX        = 0x35
	absMtY        = 0x36
	absMtTracking = 0x39
	btnTouch      = 0x14a
	keyPower      = 116
	keyPageUp     = 104
	keyPageDown   = 109
	eviocgrab     = 0x40044590
	eviocgabsBase = 0x80184540
	swipeDistance = 80
)

type gestureKind int

const (
	gestureTap gestureKind = iota
	gestureSwipe
)

type gesture struct {
	kind gestureKind
	x    int
	y    int
	dy   int
}

type keyPress struct {
	code int
}

type touchMapping struct {
	swapAxes bool
	mirrorX  bool
	mirrorY  bool
	width    int
	height   int
}

type absInfo struct {
	Value, Minimum, Maximum, Fuzz, Flat, Resolution int32
}

func ioctl(args ioctlArgs) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, args.fd, args.request, args.arg)
	if errno != 0 {
		return errno
	}
	return nil
}

type ioctlArgs struct {
	fd      uintptr
	request uintptr
	arg     uintptr
}

func absMaximum(args absMaximumArgs) int {
	info := absInfo{}
	ioctl(ioctlArgs{fd: args.file.Fd(), request: eviocgabsBase + args.axis, arg: uintptr(unsafe.Pointer(&info))})
	return int(info.Maximum)
}

type absMaximumArgs struct {
	file *os.File
	axis uintptr
}

func grab(file *os.File) func() {
	ioctl(ioctlArgs{fd: file.Fd(), request: eviocgrab, arg: 1})
	return func() { ioctl(ioctlArgs{fd: file.Fd(), request: eviocgrab, arg: 0}) }
}

func (m touchMapping) toScreen(args touchPoint) (int, int) {
	x, y := args.x*m.width/max(args.maxX, 1), args.y*m.height/max(args.maxY, 1)
	if m.swapAxes {
		x, y = args.y*m.width/max(args.maxY, 1), args.x*m.height/max(args.maxX, 1)
	}
	if m.mirrorX {
		x = m.width - 1 - x
	}
	if m.mirrorY {
		y = m.height - 1 - y
	}
	return min(max(x, 0), m.width-1), min(max(y, 0), m.height-1)
}

type touchPoint struct {
	x, y, maxX, maxY int
}

func readTouch(args readTouchArgs) {
	file, err := os.Open(args.path)
	if err != nil {
		args.errors <- err
		return
	}
	defer file.Close()
	release := grab(file)
	defer release()
	maxX, maxY := absMaximum(absMaximumArgs{file: file, axis: absMtX}), absMaximum(absMaximumArgs{file: file, axis: absMtY})
	buffer := make([]byte, 16)
	rawX, rawY := 0, 0
	down, moved := false, false
	startX, startY, lastX, lastY := 0, 0, 0, 0
	for {
		if _, err := file.Read(buffer); err != nil {
			args.errors <- err
			return
		}
		kind := binary.LittleEndian.Uint16(buffer[8:])
		code := binary.LittleEndian.Uint16(buffer[10:])
		value := int(int32(binary.LittleEndian.Uint32(buffer[12:])))
		switch {
		case kind == evAbs && code == absMtX:
			rawX = value
			moved = true
		case kind == evAbs && code == absMtY:
			rawY = value
			moved = true
		case kind == evKey && code == btnTouch && value == 1:
			down = true
			lastX = -1
		case kind == evKey && code == btnTouch && value == 0:
			down = false
			dy := lastY - startY
			if abs(dy) >= swipeDistance && abs(dy) > abs(lastX-startX) {
				args.gestures <- gesture{kind: gestureSwipe, x: startX, y: startY, dy: dy}
			} else {
				args.gestures <- gesture{kind: gestureTap, x: startX, y: startY}
			}
		case kind == evSyn && code == 0 && moved:
			moved = false
			x, y := args.mapping.toScreen(touchPoint{x: rawX, y: rawY, maxX: maxX, maxY: maxY})
			if down && lastX == -1 {
				startX, startY = x, y
			}
			lastX, lastY = x, y
		}
	}
}

type readTouchArgs struct {
	path     string
	mapping  touchMapping
	gestures chan<- gesture
	errors   chan<- error
}

func readKeys(args readKeysArgs) {
	file, err := os.Open(args.path)
	if err != nil {
		log.Println("keys:", args.path, err)
		return
	}
	log.Println("keys: reading", args.path)
	defer file.Close()
	release := grab(file)
	defer release()
	buffer := make([]byte, 16)
	for {
		if _, err := file.Read(buffer); err != nil {
			log.Println("keys:", args.path, err)
			return
		}
		kind := binary.LittleEndian.Uint16(buffer[8:])
		code := binary.LittleEndian.Uint16(buffer[10:])
		value := int32(binary.LittleEndian.Uint32(buffer[12:]))
		if kind != evSyn {
			log.Println("input:", args.path, "type", kind, "code", code, "value", value)
		}
		if kind == evKey && value == 1 {
			args.keys <- keyPress{code: int(code)}
		}
	}
}

type readKeysArgs struct {
	path string
	keys chan<- keyPress
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
