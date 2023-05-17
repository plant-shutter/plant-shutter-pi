package schedule

import (
	"log"
	"testing"
)

func Test(t *testing.T) {
	list := []int{1, 2, 3}
	i := 1
	list = append(list[:i], list[i+1:]...)
	log.Println(list)
}
