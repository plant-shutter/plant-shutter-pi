package main

import (
	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
	"log"
)

func main() {
	devName := "/dev/video0"
	dev, err := device.Open(
		devName,
	)
	if err != nil {
		panic(err)
	}
	sizes, err := v4l2.GetAllFormatFrameSizes(dev.Fd())
	if err != nil {
		panic(err)
	}
	for _, size := range sizes {
		if size.PixelFormat == v4l2.PixelFmtJPEG {
			log.Println(size)
		}
	}
	//marshal, err := json.MarshalIndent(info, "", "    ")
	//if err != nil {
	//	panic(err)
	//}
	//fmt.Println(string(marshal))
}
