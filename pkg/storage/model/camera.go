package model

import (
	"fmt"

	"github.com/goccy/go-json"
)

//objectbox:skip
type CameraSettings map[uint32]int32

func CameraSettingsConvToDatabaseValue(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte{}, nil
	}
	t, ok := v.(CameraSettings)
	if !ok {
		return nil, fmt.Errorf("CameraSettingsConv.ToDatabase: want CameraSettings, got %T", v)
	}
	b, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	return b, nil // 返回给 OBX 存到 bytevector
}

func CameraSettingsConvToEntityProperty(v interface{}) (CameraSettings, error) {
	var b []byte
	switch x := v.(type) {
	case []byte:
		b = x
	case string:
		b = []byte(x)
	default:
		return nil, fmt.Errorf("CameraSettingsConv.FromDatabase: unexpected %T", v)
	}
	if len(b) == 0 {
		return CameraSettings{}, nil
	}
	var t CameraSettings
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, err
	}
	return t, nil
}
