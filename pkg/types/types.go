package types

import (
	"time"
)

type File struct {
	Name    string    `json:"name"`
	Size    string    `json:"size"`
	ModTime time.Time `json:"modTime"`
}
