package types

import (
	"time"
)

type VideoSetting struct {
	Enable             bool    `json:"enable"`
	FPS                int     `json:"fps"`
	MaxImage           int     `json:"maxImage"`
	ShootingDays       float32 `json:"shootingDays"`
	TotalVideoLength   float32 `json:"totalVideoLength"`
	PreviewVideoLength float32 `json:"previewVideoLength"`
}

type CameraSettings map[int32]int32

type File struct {
	Name    string    `json:"name"`
	Size    string    `json:"size"`
	ModTime time.Time `json:"modTime"`
}
