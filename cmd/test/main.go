package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"

	"plant-shutter-pi/pkg/utils"
)

var (
	devName = "/dev/video0"
)

func main() {
	sizes := []Size{
		//{
		//	Width:  640,
		//	Height: 480,
		//},
		//{
		//	Width:  1280,
		//	Height: 720,
		//},
		//{
		//	Width:  1920,
		//	Height: 1080,
		//},
		//{
		//	Width:  2560,
		//	Height: 1440,
		//},
		{
			Width:  3280,
			Height: 2464,
		},
	}
	for _, s := range sizes {
		if err := shot(s.Width, s.Height); err != nil {
			panic(err)
		}
		time.Sleep(time.Second * 5)
	}
}

type Size struct {
	Width  int
	Height int
}

func shot(width, height int) error {
	log.Printf("shot %d*%d", width, height)
	dev, err := device.Open(
		devName,
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtRGB24, Width: uint32(width), Height: uint32(height)}),
	)
	if err != nil {
		return err
	}
	defer func(dev *device.Device) {
		err := dev.Close()
		if err != nil {
			log.Println(err)
		}
	}(dev)

	pixFormat, err := dev.GetPixFormat()
	if err != nil {
		return err
	}
	log.Println(pixFormat)
	pixFormat, err = v4l2.GetPixFormat(dev.Fd())
	if err != nil {
		return err
	}
	log.Println(pixFormat)

	// start stream
	if err = dev.Start(context.TODO()); err != nil {
		return err
	}
	for i := 0; i < 10; i++ {
		//t1 := time.Now()
		frame := <-dev.GetOutput()
		//t2 := time.Now()
		img := utils.DecodeRGB(frame, int(pixFormat.Width), int(pixFormat.Height))
		//t3 := time.Now()
		if err = utils.EncodeJPEGFile(img, fmt.Sprintf("%d-%d.jpg", width, height), 95); err != nil {
			return err
		}
		//t4 := time.Now()

		//d1 := t2.Sub(t1)
		//d2 := t3.Sub(t2)
		//d3 := t4.Sub(t3)
		//dA := t4.Sub(t1)
		//log.Println(d1, d2, d3, dA)
	}

	//err = os.WriteFile(fmt.Sprintf("%d-%d.raw", width, height), frame, 0640)
	//if err != nil {
	//	return err
	//}

	return nil
}
