package main

import (
	"context"
	"log"
	"plant-shutter-pi/pkg/utils"
	"plant-shutter-pi/pkg/utils/rgb"

	"github.com/vladimirvivien/go4vl/v4l2"

	"plant-shutter-pi/pkg/camera"
)

func main() {
	err := camera.Init(camera.DefaultDevice)
	if err != nil {
		log.Fatalln(err)
	}
	defer camera.Close()

	// todo: test start -> close -> start -> close
	//err = camera.dev.SetPixFormat(v4l2.PixFormat{
	//	Width:  640,
	//	Height: 480,
	//})
	//if err != nil {

	//}

	if err = getImage("1.jpg"); err != nil {
		log.Println(err)
		return
	}
	if err = getImage("2.jpg"); err != nil {
		log.Println(err)
		return
	}
}

func getImage(path string) error {
	format, err := v4l2.GetPixFormat(camera.GetDev().Fd())
	if err != nil {
		return err
	}

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	if err = camera.Start(ctx); err != nil {
		return err
	}

	log.Println("get output")

	f := <-camera.GetOutput()
	img := rgb.NewRGB(f, int(format.Width), int(format.Height))
	if err = utils.EncodeJPEGFile(img, path, 95); err != nil {
		return err
	}

	return nil
}
