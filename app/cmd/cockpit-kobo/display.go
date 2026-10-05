package main

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

type varScreenInfo struct {
	Xres, Yres, XresVirtual, YresVirtual, Xoffset, Yoffset, BitsPerPixel, Grayscale uint32
	Bitfields                                                                        [12]uint32
	Rest                                                                             [20]uint32
}

type fixScreenInfo struct {
	ID         [16]byte
	SmemStart  uint32
	SmemLen    uint32
	Type       uint32
	TypeAux    uint32
	Visual     uint32
	XpanStep   uint16
	YpanStep   uint16
	YwrapStep  uint16
	LineLength uint32
	Rest       [5]uint32
}

type refreshMode int

const (
	refreshPartial refreshMode = iota
	refreshFull
)

type display struct {
	file     *os.File
	memory   []byte
	width    int
	height   int
	stride   int
	base     int
	previous []byte
	fbink    string
}

func openDisplay(fbink string) (*display, error) {
	file, err := os.OpenFile("/dev/fb0", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	vinfo := varScreenInfo{}
	finfo := fixScreenInfo{}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), 0x4600, uintptr(unsafe.Pointer(&vinfo))); errno != 0 {
		file.Close()
		return nil, errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), 0x4602, uintptr(unsafe.Pointer(&finfo))); errno != 0 {
		file.Close()
		return nil, errno
	}
	if vinfo.BitsPerPixel != 32 {
		file.Close()
		return nil, fmt.Errorf("unsupported framebuffer depth: %d bpp", vinfo.BitsPerPixel)
	}
	memory, err := syscall.Mmap(int(file.Fd()), 0, int(finfo.SmemLen), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, err
	}
	return &display{
		file:   file,
		memory: memory,
		width:  int(vinfo.Xres),
		height: int(vinfo.Yres),
		stride: int(finfo.LineLength),
		base:   int(vinfo.Yoffset)*int(finfo.LineLength) + int(vinfo.Xoffset)*4,
		fbink:  fbink,
	}, nil
}

func (d *display) close() {
	syscall.Munmap(d.memory)
	d.file.Close()
}

func (d *display) present(args presentArgs) error {
	img := args.image
	rowBytes := d.width * 4
	top, bottom := -1, -1
	for y := 0; y < d.height; y++ {
		source := img.Pix[y*img.Stride : y*img.Stride+rowBytes]
		if d.previous != nil && !args.force && string(source) == string(d.previous[y*rowBytes:(y+1)*rowBytes]) {
			continue
		}
		offset := d.base + y*d.stride
		copy(d.memory[offset:offset+rowBytes], source)
		if top < 0 {
			top = y
		}
		bottom = y
	}
	if d.previous == nil {
		d.previous = make([]byte, rowBytes*d.height)
	}
	for y := 0; y < d.height; y++ {
		copy(d.previous[y*rowBytes:(y+1)*rowBytes], img.Pix[y*img.Stride:y*img.Stride+rowBytes])
	}
	if top < 0 {
		return nil
	}
	if args.mode == refreshFull {
		return exec.Command(d.fbink, "-q", "-f", "-W", "GC16", "-s").Run()
	}
	region := fmt.Sprintf("top=%d,left=0,width=%d,height=%d", top, d.width, bottom-top+1)
	return exec.Command(d.fbink, "-q", "-W", "AUTO", "-s", region).Run()
}

type presentArgs struct {
	image *image.RGBA
	mode  refreshMode
	force bool
}
