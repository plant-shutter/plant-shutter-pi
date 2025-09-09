package main

import (
	"log"
	"time"

	"github.com/objectbox/objectbox-go/objectbox"
	"plant-shutter-pi/pkg/storage/model"
)

func initObjectBox() *objectbox.ObjectBox {
	objectBox, err := objectbox.NewBuilder().Model(model.ObjectBoxModel()).Build()
	if err != nil {
		panic(err)
	}
	return objectBox
}

func main() {
	// load objectbox
	ob := initObjectBox()
	defer ob.Close() // In a server app, you would just keep ob and close on shutdown

	box := model.BoxForProjectEntity(ob)

	// Create
	id, _ := box.Put(&model.ProjectEntity{
		Name:      "test",
		CreatedAt: time.Now(),
		CameraSettings: model.CameraSettings{
			1: 1,
		},
	})

	task, _ := box.Get(id) // Read
	log.Println(task.Name, task.CreatedAt, task.CameraSettings)
	task.Name += " & some bread"
	box.Put(task)         // Update
	task, _ = box.Get(id) // Read
	log.Println(task.Name, task.CreatedAt, task.CameraSettings)

	box.Remove(task)         // Delete
	task, err := box.Get(id) // Read
	log.Println(task, err)
}
