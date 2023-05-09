package image

import "time"

type ImagesInfo struct {
	Name      string
	MaxNumber int
	UpdateAt  time.Time
	Total     int
}
