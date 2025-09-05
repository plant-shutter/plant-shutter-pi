package camera

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// Controller 使用持久的预览通道来管理预览与拍照。
//
// 行为：
//   - StartPreview(width,height) 以给定分辨率启动设备，
//     并返回 JPEG 帧的通道。该通道在预览生命周期内保持不变；
//     StopPreview 会将其关闭。
//   - Capture(width,height) 拍摄单张照片。若预览正在运行，
//     将临时停止设备，切换到拍照分辨率获取一帧，随后恢复至预览分辨率。
//     拍照期间预览通道保持打开，但不会收到帧。
type Controller struct {
	mu sync.Mutex

	cam *Camera

	// 对外暴露的预览通道（首次 StartPreview 时创建）
	previewCh chan []byte

	// 预览循环控制
	loopStop  chan struct{}
	srcUpdate chan (<-chan []byte) // 供循环更新当前源流

	// 当前预览分辨率（用于拍照后恢复）
	pW, pH int

	// 状态标志
	previewing bool
}

// NewController 创建一个绑定到设备路径的控制器。
func NewController(cam *Camera) *Controller {
	return &Controller{cam: cam}
}

// StartPreview 以 width x height 启动预览并返回预览通道。
// 如果预览已在运行，则返回错误。
func (c *Controller) StartPreview(width, height int) (<-chan []byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.previewing {
		return nil, errors.New("preview already started")
	}

	if c.previewCh == nil {
		c.previewCh = make(chan []byte, 1)
		c.loopStop = make(chan struct{})
		c.srcUpdate = make(chan (<-chan []byte), 1)
		go c.previewLoop()
	}

	frames, err := c.cam.Start(width, height)
	if err != nil {
		return nil, err
	}
	c.pW, c.pH = width, height
	c.previewing = true

	c.srcUpdate <- frames

	return c.previewCh, nil
}

// StopPreview 停止预览并关闭预览通道。
func (c *Controller) StopPreview() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.previewing && c.previewCh == nil {
		return nil
	}
	c.previewing = false
	// 先停止设备，使循环停止接收帧
	_ = c.cam.Stop()
	// 通知循环退出并关闭预览通道
	if c.loopStop != nil {
		close(c.loopStop)
		c.loopStop = nil
	}
	// 允许重新启动：重置更新通道；previewCh 将由循环关闭
	c.srcUpdate = nil
	c.previewCh = nil

	return nil
}

// Capture 以 width x height 捕获一帧并返回 []byte。
// 若预览正在运行，拍照期间预览通道保持打开但暂停发送，之后自动恢复。
func (c *Controller) Capture(width, height int) ([]byte, error) {
	// 在锁内决定状态切换
	c.mu.Lock()
	wasPreviewing := c.previewing
	// 切换到拍照状态
	c.previewing = false
	c.mu.Unlock()

	if wasPreviewing {
		// 停止当前预览流；预览循环将暂停（无源）
		_ = c.cam.Stop()
	}

	// 以请求的分辨率启动拍照流
	frames, err := c.cam.Start(width, height)
	if err != nil {
		// 失败时尝试恢复预览状态
		if wasPreviewing {
			if fr, e2 := c.resumePreview(c.pW, c.pH); e2 == nil {
				c.mu.Lock()
				c.previewing = true
				c.mu.Unlock()
				// 刷新预览源
				c.srcUpdate <- fr
			} else {
				logger.Warnf("failed to resume preview after capture start error: %v", e2)
			}
		}
		return nil, err
	}

	// 读取一帧
	var img []byte
	frame, ok := <-frames
	if !ok {
		_ = c.cam.Stop()
		// 如需则恢复预览
		if wasPreviewing {
			if fr, e2 := c.resumePreview(c.pW, c.pH); e2 == nil {
				c.mu.Lock()
				c.previewing = true
				c.mu.Unlock()
				c.srcUpdate <- fr
			}
		}
		return nil, errors.New("capture stream closed")
	}
	if len(frame) > 0 {
		img = make([]byte, len(frame))
		copy(img, frame)
	} else {
		img = []byte{}
	}

	// 停止拍照流
	_ = c.cam.Stop()

	// 若之前在预览则恢复
	if wasPreviewing {
		fr, err2 := c.resumePreview(c.pW, c.pH)
		if err2 != nil {
			logger.Warnf("failed to resume preview after capture: %v", err2)
			return img, nil // return captured image anyway
		}
		c.mu.Lock()
		c.previewing = true
		c.mu.Unlock()
		// 更新预览循环使用的源
		c.srcUpdate <- fr
	}

	return img, nil
}

// previewLoop 将当前源的帧复用转发到 previewCh。
// 它会在临时停止（如拍照）期间保持 previewCh 打开，
// 仅当调用 StopPreview 时才关闭。
func (c *Controller) previewLoop() {
	var current <-chan []byte
	for {
		// 若当前无源，等待更新或停止信号
		if current == nil {
			select {
			case <-c.loopStop:
				// 关闭对外通道并退出
				c.mu.Lock()
				if c.previewCh != nil {
					close(c.previewCh)
				}
				c.mu.Unlock()
				return
			case ch := <-c.srcUpdate:
				current = ch
			}
			continue
		}

		select {
		case <-c.loopStop:
			c.mu.Lock()
			if c.previewCh != nil {
				close(c.previewCh)
			}
			c.mu.Unlock()
			return
		case ch := <-c.srcUpdate:
			// 切换到新源
			current = ch
		case frame, ok := <-current:
			if !ok {
				// 源已结束（如用于拍照）；暂停直至新源到来
				current = nil
				continue
			}
			if frame == nil {
				continue
			}
			// 非阻塞转发；若消费者处理慢则丢弃
			select {
			case c.previewCh <- append([]byte(nil), frame...):
			default:
				// 为避免阻塞而丢帧
			}
		}
	}
}

// resumePreview 尝试在拍照结束后恢复预览；针对驱动短暂的 EBUSY 加入重试。
func (c *Controller) resumePreview(width, height int) (<-chan []byte, error) {
	// 给底层释放留一点时间
	time.Sleep(50 * time.Millisecond)
	var (
		fr  <-chan []byte
		err error
	)
	for i := 0; i < 5; i++ {
		fr, err = c.cam.Start(width, height)
		if err == nil {
			return fr, nil
		}
		if !isBusyErr(err) {
			break
		}
		logger.Warnf("failed to resume preview will retry %d/5: %v", i+1, err)
		time.Sleep(150 * time.Millisecond)
	}
	return nil, err
}

func isBusyErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "busy") || strings.Contains(s, "ebusy")
}
