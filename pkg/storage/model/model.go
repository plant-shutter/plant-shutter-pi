package model

import (
	"time"
)

//go:generate go run github.com/objectbox/objectbox-go/cmd/objectbox-gogen

// ProjectEntity is the ObjectBox entity representing a project configuration.
type ProjectEntity struct {
	Id       uint64 `objectbox:"id" json:"id,omitempty"`
	Name     string `objectbox:"unique" json:"name,omitempty"`
	Info     string `json:"info,omitempty"`
	Interval int32  `json:"interval,omitempty"`
	// Capture metadata only; frames are stored as individual JPEG files.
	ImageCount      int    `json:"imageCount,omitempty"`
	LatestImageName string `json:"latestImageName,omitempty"`
	// Camera settings
	CameraSettings CameraSettings `objectbox:"type:[]byte converter:CameraSettingsConv" json:"camera,omitempty"`

	StartedAt time.Time `objectbox:"date" json:"startedAt"`
	EndedAt   time.Time `objectbox:"date" json:"endedAt"`

	UpdateAt  time.Time `objectbox:"date" json:"updatedAt"`
	CreatedAt time.Time `objectbox:"date" json:"createdAt"`
}

// LastRunningEntity stores the name of the last-running project.
// Using a single-row box keeps the logic simple.
type LastRunningEntity struct {
	Id          uint64 `objectbox:"id"`
	ProjectName string
}
