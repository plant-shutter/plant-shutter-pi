package camera

import (
	"context"
	"fmt"
	"log"
	"testing"

	"plant-shutter-pi/pkg/utils/image"
)

func TestCamera(t *testing.T) {
	err := Init(DefaultDevice)
	if err != nil {
		log.Fatalln(err)
	}
	defer Close()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	err = Start(ctx)
	if err != nil {
		log.Fatalln(err)
	}

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

	for _, size := range sizes {
		if err = SetPixFormat(size.Width, size.Height); err != nil {
			log.Fatalln(err)
		}
		f := <-GetOutput()
		img := image.DecodeRGB(f, size.Width, size.Height)

		if err = image.EncodeJPEGFile(img, size.String()+".jpg", 95); err != nil {
			log.Fatalln(err)
		}
	}
}

type Size struct {
	Width  int
	Height int
}

func (s Size) String() string {
	return fmt.Sprintf("%d_%d", s.Width, s.Height)
}
