package main

import "fmt"
import "encoding/binary"

func main() {
	var mySlice = []byte{255, 255, 255, 255, 255, 255, 255, 255}
	data := binary.BigEndian.Uint64(mySlice)
	fmt.Println(data)
	fmt.Println(int64(data))
}
