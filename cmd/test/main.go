package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

func main() {
	devName := "/dev/video0"
	flag.StringVar(&devName, "d", devName, "dev name (path)")
	flag.Parse()

	// open dev
	dev, err := device.Open(
		devName,
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtRGB24, Width: 3280, Height: 2464}),
	)
	if err != nil {
		log.Fatalf("failed to open dev: %s", err)
	}
	defer dev.Close()
	// start stream
	ctx, stop := context.WithCancel(context.TODO())
	if err := dev.Start(ctx); err != nil {
		log.Fatalf("failed to start stream: %s", err)
	}
	t1 := time.Now()
	frame := <-dev.GetOutput()
	t2 := time.Now()

	fileName := fmt.Sprintf("raw1")
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := file.Write(frame); err != nil {
		log.Fatal(err)
	}
	log.Printf("Saved file: %s", fileName)
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
	t3 := time.Now()

	img := image.NewRGBA(image.Rect(0, 0, 3280, 2464))
	img.Pix = frame

	if err := writeImage(img, "out.png"); err != nil {
		log.Println(err)
	}
	log.Println(t2.Sub(t1))
	log.Println(t3.Sub(t2))

	stop() // stop capture
	fmt.Println("Done.")

}
func writeImage(img image.Image, name string) error {
	fd, err := os.Create(name)
	if err != nil {
		return err
	}
	defer fd.Close()

	return png.Encode(fd, img)
}
