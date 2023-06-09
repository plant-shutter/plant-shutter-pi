package ov

import (
	"github.com/vladimirvivien/go4vl/v4l2"

	"plant-shutter-pi/pkg/types"
)

type NewProject struct {
	Name     string             `json:"name" binding:"required"`
	Info     string             `json:"info" binding:"required"`
	Interval int                `json:"interval" binding:"required"`
	Video    types.VideoSetting `json:"video"`
}

type UpdateProject struct {
	Name     string  `json:"name" binding:"required"`
	Info     *string `json:"info"`
	Interval *int    `json:"interval"`
	Running  *bool   `json:"running"`
}

type ProjectName struct {
	Name string `json:"name" binding:"required"`
}

type Config struct {
	// ID 公认的值，是唯一的，和ble的uid有点像
	ID v4l2.CtrlID `json:"ID"`
	// 当前值
	Value v4l2.CtrlValue `json:"value"`
	// 人类可读的名称，直接从摄像头获取的，所以是英文
	Name string `json:"name"`

	// 是否为菜单
	IsMenu bool `json:"isMenu"`

	// 如果是菜单，那就有Items，下标-人类可读名称
	// map[index]name
	MenuItems map[uint32]string `json:"menuItems"`

	// 最小值
	Minimum int32 `json:"minimum"`
	// 最大值
	Maximum int32 `json:"maximum"`
	//步进值
	Step int32 `json:"step"`
}

type UpdateConfig struct {
	ID    v4l2.CtrlID
	Value v4l2.CtrlValue
}
