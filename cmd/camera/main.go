package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

var (
	frames <-chan []byte
)

func imageServ(w http.ResponseWriter, req *http.Request) {
	mimeWriter := multipart.NewWriter(w)
	w.Header().Set("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", mimeWriter.Boundary()))
	partHeader := make(textproto.MIMEHeader)
	partHeader.Add("Content-Type", "image/jpeg")

	var frame []byte
	start := time.Now()
	for frame = range frames {
		end := time.Now()
		log.Println(end.Sub(start))
		start = end
		partWriter, err := mimeWriter.CreatePart(partHeader)
		if err != nil {
			log.Printf("failed to create multi-part writer: %s", err)
			return
		}

		if _, err := partWriter.Write(frame); err != nil {
			log.Printf("failed to write image: %s", err)
		}
	}
}

func main() {
	port := ":9090"
	devName := "/dev/video0"
	flag.StringVar(&devName, "d", devName, "device name (path)")
	flag.StringVar(&port, "p", port, "webcam service port")

	camera, err := device.Open(
		devName,
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtJPEG, Width: 1280, Height: 720}),
	)
	if err = setDevice(camera); err != nil {
		log.Fatalf("failed to open device: %s", err)
	}
	if err != nil {
		log.Fatalf("failed to open device: %s", err)
	}
	defer camera.Close()

	if err := camera.Start(context.TODO()); err != nil {
		log.Fatalf("camera start: %s", err)
	}

	frames = camera.GetOutput()

	log.Printf("Serving images: [%s/stream]", port)
	http.HandleFunc("/stream", imageServ)
	log.Fatal(http.ListenAndServe(port, nil))
}

func setDevice(dev *device.Device) error {
	ctrls, err := v4l2.QueryAllExtControls(dev.Fd())
	if err != nil {
		return err
	}

	for _, ctrl := range ctrls {
		if ctrl.Name == "Compression Quality" {
			if err = dev.SetControlValue(ctrl.ID, 90); err != nil {
				return err
			}
			//control, err := dev.GetControl(ctrl.ID)
			//if err != nil {
			//	return err
			//}
			//log.Println(control.Value)
		}
	}

	return nil
}
