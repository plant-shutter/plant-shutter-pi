package camera

import (
	"context"
	"errors"
	"sync"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

type Camera struct {
	devName string
	camera  *device.Device
	lock    sync.Mutex
	cancel  context.CancelFunc
}

func New(devName string) *Camera {
	return &Camera{devName: devName}
}

func (c *Camera) Start(ctx context.Context, width, height int) (<-chan []byte, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.camera != nil {
		return nil, errors.New("already started")
	}
	newCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	camera, err := device.Open(
		c.devName,
		device.WithBufferSize(1),
		device.WithPixFormat(v4l2.PixFormat{
			PixelFormat: v4l2.PixelFmtJPEG,
			Width:       uint32(width),
			Height:      uint32(height),
		}),
	)
	if err != nil {
		return nil, err
	}
	if err = camera.Start(newCtx); err != nil {
		return nil, err
	}
	c.camera = camera
	return camera.GetOutput(), nil
}

func (c *Camera) Stop() error {
	if c.camera != nil {
		c.cancel()
		err := c.camera.Close()
		c.camera = nil
		return err
	}
	return nil
}
