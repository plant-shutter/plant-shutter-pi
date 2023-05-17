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
	pixFormat          v4l2.PixFormat

	dev  *device.Device
	lock sync.Mutex
)

func Init(devName string) error {
	var err error
	dev, err = device.Open(
		devName,
		device.WithFPS(DefaultFPS),
	)
	pixFormat, err = v4l2.GetPixFormat(dev.Fd())
	if err != nil {
		return err
	}

	return err
}

func GetDev() *device.Device {
	return dev
}

func SetPixFormat(width, height int) error {
	err := dev.SetPixFormat(v4l2.PixFormat{
		Width:  uint32(width),
		Height: uint32(height),
		Field:  v4l2.FieldNone,
	})
	if err != nil {
		return err
	}
	pixFormat, err = v4l2.GetPixFormat(dev.Fd())

	return err
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

func getSizes() error {
	frameSizes, err := v4l2.GetFormatFrameSizes(dev.Fd(), DefaultPixelFormat)
	if err != nil {
		return err
	}
	for _, size := range frameSizes {
		log.Println(size)
	}

	return nil
}
