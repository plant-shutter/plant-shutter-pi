package camera

import (
	"fmt"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
	"go.uber.org/zap"

	"plant-shutter-pi/pkg/utils"
)

var logger *zap.SugaredLogger

func init() {
	logger = utils.GetLogger()
}

func InitControls(dev *device.Device) error {
	ctrls, err := v4l2.QueryAllExtControls(dev.Fd())
	if err != nil {
		return err
	}
	ctrlSettings := map[v4l2.CtrlID]v4l2.CtrlValue{
		10094849: 1,    // Auto Exposure: Auto Mode
		10094850: 3000, // Exposure Time, Absolute: 1000
		10094868: 0,    // White Balance, Auto & Preset: Manual
		10291459: 90,   // Compression Quality: 90
		10094872: 0,    // ISO Sensitivity, Auto: Manual
	}
	for _, ctrl := range ctrls {
		if value, ok := ctrlSettings[ctrl.ID]; ok {
			if err := dev.SetControlValue(ctrl.ID, value); err != nil {
				return err
			}
			logger.Infof("set ctrl(%s) to %d", ctrl.Name, value)
		}
	}

	return nil
}

func CtrlToString(ctrl v4l2.Control) string {
	return fmt.Sprintf("Control id (%d) name: %s\t[min: %d; max: %d; step: %d; default: %d current_val: %d]\n",
		ctrl.ID, ctrl.Name, ctrl.Minimum, ctrl.Maximum, ctrl.Step, ctrl.Default, ctrl.Value)
}
