package utils

import (
	"os"
	"testing"
)

func Test(t *testing.T) {
	file, err := os.ReadFile("C:\\Users\\85761\\repo\\plant-shutter-pi\\plant-shutter\\raw.640")
	if err != nil {
		t.Fatal(err)
	}
	img := DecodeRGB(file, 640, 480)
	err = EncodeJPEGFile(img, "./t.jpg", 95)
	if err != nil {
		t.Fatal(err)
	}
}
