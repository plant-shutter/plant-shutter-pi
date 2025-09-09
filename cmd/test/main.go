package main

import (
	"log"

	"github.com/objectbox/objectbox-go/objectbox"
	"plant-shutter-pi/pkg/model"
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

	box := model.BoxForTask(ob)

	// Create
	id, _ := box.Put(&model.Task{
		Text: "Buy milk",
	})

	task, _ := box.Get(id) // Read
	log.Println(task.Text)
	task.Text += " & some bread"
	box.Put(task)         // Update
	task, _ = box.Get(id) // Read
	log.Println(task.Text)

	box.Remove(task) // Delete
}
