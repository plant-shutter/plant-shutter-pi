package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
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
		//	Width:  1920,
		//	Height: 1080,
		//},
		//{
		//	Width:  2048,
		//	Height: 1080,
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
	}
}

type Size struct {
	Width  int
	Height int
}

func shot(width, height int) error {
	dev, err := device.Open(
		devName,
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: uint32(width), Height: uint32(height)}),
		device.WithBufferSize(0),
	)
	if err != nil {
		return err
	}
	defer dev.Close()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	// start stream
	if err = dev.Start(ctx); err != nil {
		return err
	}

	for i := 0; i < 5; i++ {
		t1 := time.Now()
		frame := <-dev.GetOutput()
		t2 := time.Now()

		err = os.WriteFile(fmt.Sprintf("%d-%d.jpg", width, height), frame, 0640)
		if err != nil {
			return err
		}

		t4 := time.Now()
		log.Println(t2.Sub(t1), t4.Sub(t2), t4.Sub(t1))
	}
	log.Println("end")

	return nil
}
