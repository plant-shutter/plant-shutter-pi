package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"

	"plant-shutter-pi/pkg/utils"
)

func main() {
	devName := "/dev/video0"
	flag.StringVar(&devName, "d", devName, "dev name (path)")
	flag.Parse()

	// open dev
	dev, err := device.Open(
		devName,
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtRGB24, Width: 640, Height: 480}),
	)
	if err != nil {
		log.Fatalf("failed to open dev: %s", err)
	}
	defer dev.Close()

	format, err := v4l2.GetPixFormat(dev.Fd())
	if err != nil {
		log.Println(err)
		return
	}

	// start stream
	ctx, stop := context.WithCancel(context.TODO())
	if err := dev.Start(ctx); err != nil {
		log.Fatalf("failed to start stream: %s", err)
	}
	frame := <-dev.GetOutput()

	err = os.WriteFile("raw.rgb", frame, 0660)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("DecodeYUYV.")

	log.Println(int(format.BytesPerLine), int(format.Width), int(format.Height))
	img := utils.DecodeRGB(frame, int(format.BytesPerLine), int(format.Width), int(format.Height))
	fmt.Println("DecodeYUYV.end")

	if err := writeImage(img, "out.jpg"); err != nil {
		log.Println(err)
	}

	stop() // stop capture
	fmt.Println("Done.")

}
func writeImage(img image.Image, name string) error {
	fd, err := os.Create(name)
	if err != nil {
		return err
	}
	defer fd.Close()

	return jpeg.Encode(fd, img, nil)
}
