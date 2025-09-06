package gpio

import (
	"fmt"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/host/v3"
)

// FlashPin 封装了一个 GPIO 引脚，用于拍照前后切换电平
type FlashPin struct {
	pin          gpio.PinIO
	triggerLevel gpio.Level
}

// NewFlashPin 选择一个 BCM 引脚，比如 rpi.P1_11 (BCM17)
// "11": gpio number
// "GPIO11": gpio name as defined per the bcm238x CPU driver
// "P1_23": board header P1 position 23 name as defined by the rpi board driver
func NewFlashPin(pinName string, triggerLevel bool) (*FlashPin, error) {
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("failed to init host: %v", err)
	}
	pin := gpioreg.ByName(pinName)
	if pin == nil {
		return nil, fmt.Errorf("no such pin: %s", pinName)
	}
	t := gpio.Level(triggerLevel)
	if err := pin.Out(!t); err != nil {
		return nil, fmt.Errorf("failed to init pin: %w", err)
	}
	return &FlashPin{pin: pin, triggerLevel: t}, nil
}

// BeforeCapture 拉高电平
func (c *FlashPin) BeforeCapture() error {
	if err := c.pin.Out(c.triggerLevel); err != nil {
		return fmt.Errorf("failed to set pin: %w", err)
	}
	return nil
}

// AfterCapture 拉低电平
func (c *FlashPin) AfterCapture() error {
	if err := c.pin.Out(!c.triggerLevel); err != nil {
		return fmt.Errorf("failed to set pin: %w", err)
	}
	return nil
}
