package ov

import (
	"github.com/vladimirvivien/go4vl/v4l2"
	"time"
)

type Project struct {
	Name     string        `json:"name" binding:"required"`
	Info     string        `json:"info" binding:"required"`
	Interval time.Duration `json:"interval" binding:"required"`
}

type Config struct {
	ID    v4l2.CtrlID
	Value v4l2.CtrlValue
	Name  string

	IsMenu bool

	MenuItems []string

	Minimum int32
	Maximum int32
	Step    int32
}

type UpdateConfig struct {
	ID    v4l2.CtrlID
	Value v4l2.CtrlValue
}
