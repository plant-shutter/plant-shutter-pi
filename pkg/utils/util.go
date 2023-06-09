package utils

import (
	"encoding/binary"
	"time"
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
