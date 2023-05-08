package image

import (
	"bytes"
	"log"
	"os"
	"testing"
)

const (
	width  = 3280
	height = 2464
)

func TestRGB(t *testing.T) {
	file, err := os.ReadFile("./test_case/rgb24.hex")
	if err != nil {
		t.Fatal(err)
	}
	var jpgBuf bytes.Buffer
	if err := EncodeJPEG(DecodeRGB(file, width, height), &jpgBuf, 95); err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile("rgb24.jpg", jpgBuf.Bytes(), 0660)
	if err != nil {
		t.Fatal(err)
	}
}

func TestYUYV(t *testing.T) {
	file, err := os.ReadFile("./test_case/yuyv.hex")
	if err != nil {
		t.Fatal(err)
	}
	log.Println(len(file) / height)
}
