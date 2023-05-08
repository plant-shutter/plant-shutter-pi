package camera

import (
	"context"
	"log"
	"sync"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

const (
	DefaultDevice = "/dev/video0"
	DefaultFPS    = 15
)

var (
	DefaultPixelFormat = v4l2.PixelFmtRGB24

	Dev  *device.Device
	lock sync.Mutex
)

func Init(devName string) error {
	var err error
	Dev, err = device.Open(
		devName,
		device.WithFPS(DefaultFPS),
	)

	return err
}

func SetPixFormat(width, height int) error {
	return Dev.SetPixFormat(v4l2.PixFormat{
		Width:  uint32(width),
		Height: uint32(height),
		Field:  v4l2.FieldNone,
	})
}

func Start(ctx context.Context) error {
	return Dev.Start(ctx)
}

func Close() error {
	return Dev.Close()
}

func GetOutput() <-chan []byte {
	return Dev.GetOutput()
}

func getSizes() error {
	frameSizes, err := v4l2.GetFormatFrameSizes(Dev.Fd(), DefaultPixelFormat)
	if err != nil {
		return err
	}
	for _, size := range frameSizes {
		log.Println(size)
	}

	return nil
}

func getFPS() {
	Dev.GetFrameRate()
}
