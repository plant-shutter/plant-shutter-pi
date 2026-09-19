package cameramode

import (
	"testing"

	"github.com/vladimirvivien/go4vl/v4l2"
)

type fakeControlDevice struct {
	values map[v4l2.CtrlID]v4l2.CtrlValue
}

func (d *fakeControlDevice) SetControlValue(id v4l2.CtrlID, value v4l2.CtrlValue) error {
	d.values[id] = value
	return nil
}

func (d *fakeControlDevice) GetControl(id v4l2.CtrlID) (v4l2.Control, error) {
	return v4l2.Control{ID: id, Value: d.values[id]}, nil
}

func TestConfigureH264ControlsEnablesDynamicExposureAndBrowserProfile(t *testing.T) {
	d := &fakeControlDevice{values: make(map[v4l2.CtrlID]v4l2.CtrlValue)}
	if err := configureH264Controls(d); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[v4l2.CtrlID]v4l2.CtrlValue{
		10094851:   1, // exposure_dynamic_framerate
		0x00990a6b: 0, // h264_profile: Baseline
		0x009909e2: 1, // repeat_sequence_header
	} {
		if got := d.values[id]; got != want {
			t.Fatalf("control %#x=%d, want %d", id, got, want)
		}
	}
}
