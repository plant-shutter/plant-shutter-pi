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
	// ID 公认的值，是唯一的，和ble的uid有点像
	ID v4l2.CtrlID `json:"ID,omitempty"`
	// 当前值
	Value v4l2.CtrlValue `json:"value,omitempty"`
	// 人类可读的名称，直接从摄像头获取的，所以是英文
	Name string `json:"name,omitempty"`

	// 是否为菜单
	IsMenu bool `json:"isMenu,omitempty"`

	// 如果是菜单，那就有Items，下标-人类可读名称
	// map[index]name
	MenuItems map[uint32]string `json:"menuItems,omitempty"`

	// 最小值
	Minimum int32 `json:"minimum,omitempty"`
	// 最大值
	Maximum int32 `json:"maximum,omitempty"`
	//步进值
	Step int32 `json:"step,omitempty"`
}

type UpdateConfig struct {
	ID    v4l2.CtrlID
	Value v4l2.CtrlValue
}
