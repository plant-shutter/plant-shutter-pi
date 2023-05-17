package ps

import (
	"log"
	"testing"
)

func TestPS(t *testing.T) {
	m, err := MemoryStatus()
	if err != nil {
		t.Fatal(err)
	}
	log.Println(m)

	c, err := CPUStatus()
	if err != nil {
		t.Fatal(err)
	}
	log.Println(c)

	Disks()
}
