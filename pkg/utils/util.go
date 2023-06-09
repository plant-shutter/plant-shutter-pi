package utils

import (
	"encoding/binary"
	"os"
	"time"

	"plant-shutter-pi/pkg/storage/consts"
)

func Str2int64(in string) int64 {
	b := []byte(in)
	for i := len(b); i <= 8; i++ {
		b = append(b, 0)
	}
	return int64(binary.LittleEndian.Uint64(b))
}

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
