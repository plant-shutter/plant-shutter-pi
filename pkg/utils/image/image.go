package image

import (
	"image"
	"image/jpeg"
	"io"
)

func RGBToRGBA(in, out []byte, width, height int) {
	outStride := width * 4
	inStride := len(in) / height

	for i := 0; i < height; i++ {
		oIndex := i * outStride
		iIndex := i * inStride
		for j := 0; j < width; j++ {
			out[oIndex] = in[iIndex]
			out[oIndex+1] = in[iIndex+1]
			out[oIndex+2] = in[iIndex+2]

			oIndex += 4
			iIndex += 3
		}
	}
}

func DecodeRGB(data []byte, width, height int) image.Image {
	i := image.NewRGBA(image.Rect(0, 0, width, height))
	RGBToRGBA(data, i.Pix, width, height)

	return i
}

func EncodeJPEG(img image.Image, dst io.Writer, quality int) error {
	return jpeg.Encode(dst, img, &jpeg.Options{Quality: quality})
}
