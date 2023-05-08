package main

import (
	"context"
	"log"

	"github.com/vladimirvivien/go4vl/v4l2"

	"plant-shutter-pi/pkg/camera"
)

func main() {
	err := camera.Init(camera.DefaultDevice)
	if err != nil {
		log.Fatalln(err)
	}
	defer camera.Close()

	//err = camera.Dev.SetPixFormat(v4l2.PixFormat{
	//	Width:  640,
	//	Height: 480,
	//})
	//if err != nil {
	//	log.Println(err)
	//	return
	//}

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	err = camera.Start(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	log.Println("init")

	format, err := v4l2.GetPixFormat(camera.Dev.Fd())
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("line: ", format.Width, format.Height, format.BytesPerLine, format.SizeImage)

	//f := <-camera.GetOutput()
	//img := image.DecodeRGB(f, int(format.BytesPerLine), 1920, 1080)
	//if err = image.EncodeJPEGFile(img, "t.jpg", 95); err != nil {
	//	log.Println(err)
	//}
}
