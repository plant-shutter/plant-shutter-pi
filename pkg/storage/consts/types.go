package consts

import (
	"time"
)

const (
	DefaultImagesDir       = "images"
	DefaultVideosDir       = "videos"
	DefaultInfoFile        = "info.json"
	DefaultLastRunningFile = "last.json"

	DefaultImageExt = ".jpg"

	DefaultFilePerm = 0660
	DefaultDirPerm  = 0750

	MinInterval = time.Millisecond * 500
)
