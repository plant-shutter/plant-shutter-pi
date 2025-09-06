package storage

import (
	"log"
	"testing"
)

func TestProject(t *testing.T) {
	s, err := New("plant-project")
	checkErr(t, err)
	ps, err := s.ListProjects()
	checkErr(t, err)
	for _, p := range ps {
		log.Println(p)

	}
	//project, err := s.GetLastRunningProject()
	//
	//checkErr(t, err)
	//log.Println(project)

	//project, err := s.GetProject("test")
	//checkErr(t, err)
	//log.Println(*project)
	//
	//name, err := project.LatestImage()
	//checkErr(t, err)
	//log.Println(name)
	//
	//err = project.SaveImage([]byte("1234"))
	//checkErr(t, err)
	//images, err := project.ListImages()
	//checkErr(t, err)
	//log.Println(images)
	//
	//name, err = project.LatestImage()
	//checkErr(t, err)
	//log.Println(name)
}

func checkErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
