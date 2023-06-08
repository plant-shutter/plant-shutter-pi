package main

import (
	"log"
	"time"
)

func main() {
	t := time.NewTimer(time.Second)
	go func() {
		for {
			select {
			case x := <-t.C:
				log.Println(x)
			}
		}
	}()
	time.Sleep(time.Second * 3)
	t.Stop()
	t.Stop()

	log.Println("stop")
	time.Sleep(time.Second * 3)
	t.Reset(time.Second)
	log.Println("reset")
	time.Sleep(time.Second * 3)
}
