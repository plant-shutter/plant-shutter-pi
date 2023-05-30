package utils

import (
	"image"
	"image/jpeg"
	"io"
	"os"
)

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
