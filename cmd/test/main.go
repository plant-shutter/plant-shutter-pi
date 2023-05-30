package main

import (
	"context"
	"fmt"
	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
	"os"
	"plant-shutter-pi/pkg/utils"
	"plant-shutter-pi/pkg/utils/rgb"
)

var (
	devName = "/dev/video0"
)

func main() {
	sizes := []Size{
		{
			Width:  640,
			Height: 480,
		},
		{
			Width:  1920,
			Height: 1080,
		},
		{
			Width:  2048,
			Height: 1080,
		},
		{
			Width:  3280,
			Height: 2464,
		},
	}
	for _, s := range sizes {
		if err := shot(s.Width, s.Height); err != nil {
			panic(err)
		}
	}
}

type Size struct {
	Width  int
	Height int
}

func shot(width, height int) error {
	dev, err := device.Open(
		devName,
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtRGB24, Width: uint32(width), Height: uint32(height)}),
	)
	if err != nil {
		return err
	}
	defer dev.Close()

	// start stream
	if err = dev.Start(context.TODO()); err != nil {
		return err
	}
	frame := <-dev.GetOutput()

	err = os.WriteFile(fmt.Sprintf("%d-%d.raw", width, height), frame, 0640)
	if err != nil {
		return err
	}

	img := rgb.NewRGB(frame, width, height)
	fmt.Println("DecodeYUYV.end")

	if err = utils.EncodeJPEGFile(img, fmt.Sprintf("%d-%d.jpg", width, height), 95); err != nil {
		return err
	}

	return nil
}
