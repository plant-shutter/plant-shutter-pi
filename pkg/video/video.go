package video

import (
	"github.com/icza/mjpeg"
)

type Builder struct {
	width  int
	height int
	fps    int

	cnt int
	aw  mjpeg.AviWriter
}

func NewBuilder(path string, width, height, fps int) (*Builder, error) {
	aw, err := mjpeg.New(path, int32(width), int32(height), int32(fps))
	if err != nil {
		return nil, err
	}

	return &Builder{
		width:  width,
		height: height,
		fps:    fps,
		aw:     aw,
	}, nil
}

func (b *Builder) Add(frame []byte) error {
	err := b.aw.AddFrame(frame)
	if err != nil {
		return err
	}
	b.cnt++

	return nil
}

func (b *Builder) Close() error {
	return b.aw.Close()
}

func (b *Builder) GetCnt() int {
	return b.cnt
}
