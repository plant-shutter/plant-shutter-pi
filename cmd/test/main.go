package main

import (
	"log"

	"plant-shutter-pi/pkg/storage"
)

func main() {
	s, err := storage.New("plant-project")
	if err != nil {
		log.Fatal(err)
	}
	ps, err := s.ListProjects()
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range ps {
		log.Println(p.Name)

	}
}
