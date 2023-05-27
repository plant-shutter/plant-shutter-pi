package camera

import (
	"context"
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

	dev  *device.Device
	lock sync.Mutex
)

func Init(devName string) error {
	var err error
	dev, err = device.Open(
		devName,
		device.WithFPS(DefaultFPS),
	)
	if err != nil {
		return err
	}

	return err
}

func GetDev() *device.Device {
	return dev
}

func SetPixFormat(width, height int) error {
	return dev.SetPixFormat(v4l2.PixFormat{
		Width:  uint32(width),
		Height: uint32(height),
		Field:  v4l2.FieldNone,
	})
}

func Start(ctx context.Context) error {
	return dev.Start(ctx)
}

func Close() error {
	return dev.Close()
}

func GetOutput() <-chan []byte {
	return dev.GetOutput()
}
