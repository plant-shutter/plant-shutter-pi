package utils

import "encoding/binary"

func Str2int64(in string) int64 {
	b := []byte(in)
	for i := len(b); i <= 8; i++ {
		b = append(b, 0)
	}
	return int64(binary.LittleEndian.Uint64(b))
}
