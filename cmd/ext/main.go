package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/goccy/go-json"
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
		log.Fatal(err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")
	if err := enc.Encode(configs); err != nil {
		panic(err)
	}
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

func printControl(ctrl v4l2.Control) {
	fmt.Printf("Control id (%d) name: %s\t[min: %d; max: %d; step: %d; default: %d current_val: %d]\n",
		ctrl.ID, ctrl.Name, ctrl.Minimum, ctrl.Maximum, ctrl.Step, ctrl.Default, ctrl.Value)

	if ctrl.IsMenu() {
		menus, err := ctrl.GetMenuItems()
		if err != nil {
			return
		}

		for _, m := range menus {
			fmt.Printf("\t(%d) Menu %s: [%d]\n", m.Index, m.Name, m.Value)
		}
	}
}
