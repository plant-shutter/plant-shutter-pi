package main

import (
	"fmt"
	"log"

	"plant-shutter-pi/pkg/storage/consts"
)

func main() {

	format := fmt.Sprintf("%%s-%%0%dd%%s", 3)
	log.Println(fmt.Sprintf(format, "xxxx", 1, consts.DefaultImageExt))
}
