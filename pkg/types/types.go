package types

import (
	"time"

	"github.com/vladimirvivien/go4vl/v4l2"
)

type VideoSetting struct {
	Enable             bool    `json:"enable"`
	FPS                int     `json:"fps"`
	MaxImage           int     `json:"maxImage"`
	ShootingDays       float32 `json:"shootingDays"`
	TotalVideoLength   float32 `json:"totalVideoLength"`
	PreviewVideoLength float32 `json:"previewVideoLength"`
}

type CameraSettings map[v4l2.CtrlID]v4l2.CtrlValue

type File struct {
	Name    string    `json:"name"`
	Size    string    `json:"size"`
	ModTime time.Time `json:"modTime"`
}
