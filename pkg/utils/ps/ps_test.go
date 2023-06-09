package ps

import (
	"log"
	"testing"
)

func TestPS(t *testing.T) {
	//m, err := MemoryStatus()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//log.Println(m)
	//
	//c, err := CPUStatus()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//log.Println(c)

	a, err := DirDiskUsage("C:\\Users\\85761\\repo\\plant-shutter-pi\\pkg\\types")
	if err != nil {
		panic(err)
	}
	log.Println(a)
}
