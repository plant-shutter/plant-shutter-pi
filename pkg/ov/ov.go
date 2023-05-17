package ov

import (
	"time"
)

type Project struct {
	Name     string        `json:"name" binding:"required"`
	Info     string        `json:"info" binding:"required"`
	Interval time.Duration `json:"interval" binding:"required"`
}
