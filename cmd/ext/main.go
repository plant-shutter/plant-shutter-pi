package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	dev "github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"

	"plant-shutter-pi/pkg/camera"
)

func main() {
	devName := "/dev/video0"
	flag.StringVar(&devName, "d", devName, "device name (path)")
	flag.Parse()

	device, err := dev.Open(devName)
	if err != nil {
		log.Fatalf("failed to open device: %s", err)
	}
	defer device.Close()

	configs, err := camera.GetKnownCtrlConfigs(device)
	if err != nil {
		log.Println(err)
		return
	}
	_ = prettyPrint(configs)
	err = t(device)
	if err != nil {
		log.Println(err)
		return
	}
}

func t(dev *dev.Device) error {
	err := dev.SetControlValue(10094849, 1)
	if err != nil {
		return err
	}
	err = dev.SetControlValue(10094850, 100)
	if err != nil {
		return err
	}
	control, err := v4l2.GetControl(dev.Fd(), 10094850)
	if err != nil {
		return err
	}
	err = prettyPrint(control)
	if err != nil {
		return err
	}

	err = shot(dev, 640, 480, "1")
	if err != nil {
		return err
	}

	//time.Sleep(time.Second)
	//err := dev.SetControlValue(10094849, 0)
	//if err != nil {
	//	return err
	//}
	//err = shot(dev, 640, 480, "2")
	//if err != nil {
	//	return err
	//}
	//
	//control, err := v4l2.GetControl(dev.Fd(), 10094850)
	//if err != nil {
	//	return err
	//}
	//err = prettyPrint(control)
	//if err != nil {
	//	return err
	//}

	return nil
}

func printControl1(ctrl v4l2.Control) {
	fmt.Printf("Control id (%d)(%d) name: %s\t[min: %d; max: %d; step: %d; default: %d current_val: %d]\n",
		ctrl.ID, ctrl.Type, ctrl.Name, ctrl.Minimum, ctrl.Maximum, ctrl.Step, ctrl.Default, ctrl.Value)

	if !ctrl.IsMenu() {
		return
	}
	menus, err := ctrl.GetMenuItems()
	if err != nil {
		return
	}
	switch ctrl.Type {
	case v4l2.CtrlTypeIntegerMenu:
		for _, m := range menus {
			b := []byte(m.Name)
			for i := len(b); i <= 8; i++ {
				b = append(b, 0)
			}
			fmt.Printf("\t(%d) Menu %d: [%d]\n", m.Index, int64(binary.LittleEndian.Uint64(b)), m.Value)
		}
	case v4l2.CtrlTypeMenu:
		for _, m := range menus {
			fmt.Printf("\t(%d) Menu %s: [%d]\n", m.Index, m.Name, m.Value)
		}
	}
}

func prettyPrint(in any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")

	return enc.Encode(in)
}

func shot(dev *dev.Device, width, height int, prefix string) error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	if err := dev.Start(ctx); err != nil {
		return err
	}

	frame := <-dev.GetOutput()
	err := os.WriteFile(fmt.Sprintf("%s-%d-%d.jpg", prefix, width, height), frame, 0640)
	if err != nil {
		return err
	}
	log.Println("shot 1")

	return nil
}
