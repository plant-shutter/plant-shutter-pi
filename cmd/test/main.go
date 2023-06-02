package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"

	"plant-shutter-pi/pkg/camera"
)

var (
	devName = "/dev/video0"
	dev     *device.Device
)

func main() {
	var err error
	dev, err = device.Open(
		devName,
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: uint32(640), Height: uint32(480)}),
		device.WithBufferSize(0),
	)
	if err != nil {
		panic(err)
	}
	defer dev.Close()

	sizes := []Size{
		{
			Width:  640,
			Height: 480,
		},
		{
			Width:  1280,
			Height: 720,
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
		time.Sleep(time.Second)
	}
}

type Size struct {
	Width  int
	Height int
}

func shot(width, height int) error {

	if err := camera.InitControls(dev); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	if err := dev.Start(ctx); err != nil {
		return err
	}

	frame := <-dev.GetOutput()
	err := os.WriteFile(fmt.Sprintf("%d-%d.jpg", width, height), frame, 0640)
	if err != nil {
		return err
	}
	log.Println("shot 1")

	return nil
}

//func setDevice(dev *device.Device) error {

//
//	for _, ctrl := range ctrls {
//		if ctrl.Name == "Compression Quality" {
//			if err = dev.SetControlValue(ctrl.ID, 95); err != nil {
//				return err
//			}
//			//control, err := dev.GetControl(ctrl.ID)
//			//if err != nil {
//			//	return err
//			//}
//			//log.Println(control.Value)
//		}
//	}
//
//	return nil
//}
