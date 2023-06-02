package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"

	dev "github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
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

	ctrls, err := v4l2.QueryAllExtControls(device.Fd())
	if err != nil {
		log.Fatalf("failed to get ext controls: %s", err)
	}
	if len(ctrls) == 0 {
		log.Println("Device does not have extended controls")
		os.Exit(0)
	}
	for _, ctrl := range ctrls {
		if ctrl.Name == "Compression Quality" {
			if err := device.SetControlValue(ctrl.ID, 95); err != nil {
				log.Println(err)
			}
		}
		printControl(ctrl)
	}
}

func printControl(ctrl v4l2.Control) {
	fmt.Printf("Control id (%d) name: %s\t[min: %d; max: %d; step: %d; default: %d current_val: %d]\n",
		ctrl.ID, ctrl.Name, ctrl.Minimum, ctrl.Maximum, ctrl.Step, ctrl.Default, ctrl.Value)

	if ctrl.IsMenu() {
		menus, err := ctrl.GetMenuItems()
		if err != nil {
			return
		}
		if ctrl.Type == v4l2.CtrlTypeIntegerMenu {
			for _, m := range menus {
				d := int64(binary.BigEndian.Uint64([]byte(m.Name)))
				fmt.Printf("\t(%d) Menu %d: [%d]\n", m.Index, d, m.Value)
			}
		} else if ctrl.Type == v4l2.CtrlTypeMenu {
			for _, m := range menus {
				fmt.Printf("\t(%d) Menu %s: [%d]\n", m.Index, m.Name, m.Value)
			}
		}

	}
}
