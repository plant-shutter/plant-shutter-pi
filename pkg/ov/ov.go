package ov

import (
	"time"

	"github.com/vladimirvivien/go4vl/v4l2"
)

type Project struct {
	Name     string        `json:"name" binding:"required"`
	Info     string        `json:"info" binding:"required"`
	Interval time.Duration `json:"interval" binding:"required"`
}

type Config struct {
	ID    v4l2.CtrlID    `json:"ID,omitempty"`
	Value v4l2.CtrlValue `json:"value,omitempty"`
	Name  string         `json:"name,omitempty"`

	IsMenu bool `json:"isMenu,omitempty"`

	// map[index]name
	MenuItems map[uint32]string `json:"menuItems,omitempty"`

	Minimum int32 `json:"minimum,omitempty"`
	Maximum int32 `json:"maximum,omitempty"`
	Step    int32 `json:"step,omitempty"`
}

type UpdateConfig struct {
	ID    v4l2.CtrlID
	Value v4l2.CtrlValue
}
