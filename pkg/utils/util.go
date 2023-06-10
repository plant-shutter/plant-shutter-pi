package utils

import (
	"os"
	"time"

	"plant-shutter-pi/pkg/storage/consts"
)

func MsToDuration(i int) time.Duration {
	return time.Millisecond * time.Duration(i)
}

func MkdirAll(dirs ...string) error {
	for _, d := range dirs {
		err := os.MkdirAll(d, consts.DefaultDirPerm)
		if err != nil {
			return err
		}
	}

	return nil
}
