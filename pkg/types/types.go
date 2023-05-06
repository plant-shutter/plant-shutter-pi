package types

import "time"

type Project struct {
	Name      string
	Info      string
	CreatedAt time.Time
}

type LastImage struct {
	Name   string
	Sha    string
	Number int
}
