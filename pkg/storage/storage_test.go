package storage

import (
	"testing"
	"time"

	"plant-shutter-pi/pkg/storage/project"
)

func TestProject(t *testing.T) {
	s, err := New("tmp")
	checkErr(t, err)
	defer s.Close()
	err = s.dumpLastRunning(&project.Project{
		Name:      "123",
		Info:      "3333",
		Interval:  0,
		CreatedAt: time.Now(),
	})
	checkErr(t, err)

	//project, err := s.GetProject("test")
	//checkErr(t, err)
	//log.Println(*project)
	//
	//name, err := project.LatestImageName()
	//checkErr(t, err)
	//log.Println(name)
	//
	//err = project.SaveImage([]byte("1234"))
	//checkErr(t, err)
	//images, err := project.ListImages()
	//checkErr(t, err)
	//log.Println(images)
	//
	//name, err = project.LatestImageName()
	//checkErr(t, err)
	//log.Println(name)
}

func checkErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
