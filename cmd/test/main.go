package main

import (
	"log"

	"github.com/goccy/go-json"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

func main() {
	devName := "/dev/video0"

	dev, err := device.Open(
		devName,
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: 1280, Height: 720}),
	)
	if err != nil {
		panic(err)
	}
	info, err := v4l2.GetAllFormatDescriptions(dev.Fd())
	//info, err := v4l2.GetAllFormatFrameSizes(dev.Fd())
	if err != nil {
		panic(err)
	}
	marshal, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		panic(err)
	}
	log.Println(string(marshal))
}
