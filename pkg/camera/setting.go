package camera

import (
	"go.uber.org/zap"

	"github.com/vladimirvivien/go4vl/v4l2"

	"plant-shutter-pi/pkg/ov"
	"plant-shutter-pi/pkg/types"
	"plant-shutter-pi/pkg/utils"
)

var (
	logger       *zap.SugaredLogger
	initSettings = types.CameraSettings{
		10094849: 1, // Auto Exposure: Auto Mode
		10094868: 0, // White Balance, Auto & Preset: Manual
		10094872: 0, // ISO Sensitivity, Auto: Manual

		10291459: 90, // Compression Quality: 90
	}
	knownCtrlID = []v4l2.CtrlID{
		10094849, // Auto Exposure: Auto Mode
		10094868, // White Balance, Auto & Preset: Manual
		10094872, // ISO Sensitivity, Auto: Manual
		9963807,  // Color Effects: Set Cb/Cr

		9963776,  // Brightness
		9963777,  // Contrast
		9963778,  // Saturation
		9963790,  // Red Balance
		9963791,  // Blue Balance
		9963803,  // Sharpness
		9963818,  // Color Effects, CbCr
		10094850, // Exposure Time, Absolute
		10094871, // ISO Sensitivity
		10291459, // Compression Quality
	}
)

func init() {
	logger = utils.GetLogger()
}

func ctrlToConfig(ctrl v4l2.Control) (ov.Config, error) {
	res := ov.Config{
		ID:      ctrl.ID,
		Value:   ctrl.Value,
		Name:    ctrl.Name,
		Minimum: ctrl.Minimum,
		Maximum: ctrl.Maximum,
		Step:    ctrl.Step,
		Default: ctrl.Default,
	}
	if !ctrl.IsMenu() {
		return res, nil
	}

	res.IsMenu = true
	items, err := ctrl.GetMenuItems()
	if err != nil {
		return ov.Config{}, err
	}
	menu := make(map[uint32]string)
	for _, i := range items {
		menu[i.Index] = i.Name
	}
	res.MenuItems = menu

	return res, nil
}
