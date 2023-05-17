package util

import (
	"os"

	"plant-shutter-pi/pkg/storage/consts"
)

func MkdirAll(dirs ...string) error {
	for _, d := range dirs {
		err := os.MkdirAll(d, consts.DefaultDirPerm)
		if err != nil {
			return err
		}
	}

	return nil
}
