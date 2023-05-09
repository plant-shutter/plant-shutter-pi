package image

import (
	"image"
	"image/jpeg"
	"io"
	"log"
	"os"
)

func RGBToRGBA(in []byte, stride uint32, out []byte, width, height uint32) {
	outStride := width * 4

	for i := uint32(0); i < height; i++ {
		oIndex := i * outStride
		iIndex := i * stride
		for j := uint32(0); j < width; j++ {
			out[oIndex] = in[iIndex]
			out[oIndex+1] = in[iIndex+1]
			out[oIndex+2] = in[iIndex+2]

			oIndex += 4
			iIndex += 3
		}
	}
}

func DecodeRGB(data []byte, stride, width, height uint32) image.Image {
	i := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	log.Println(i.Stride)
	RGBToRGBA(data, stride, i.Pix, width, height)

	return i
}

func EncodeJPEG(img image.Image, dst io.Writer, quality int) error {
	return jpeg.Encode(dst, img, &jpeg.Options{Quality: quality})
}

func EncodeJPEGFile(img image.Image, file string, quality int) error {
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		return err
	}
	defer f.Close()

	return EncodeJPEG(img, f, quality)
}
