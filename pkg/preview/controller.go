package preview

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"plant-shutter-pi/pkg/cameramode"
)

var ErrProjectRunning = fmt.Errorf("capture project is running; preview unavailable")

type Controller struct {
	Manager        ModeManager
	Hub            *Broadcaster
	ProjectRunning func() bool
	mu             sync.Mutex
	mode           cameramode.Mode
	IdleTimeout    time.Duration
}

func (c *Controller) Mode() cameramode.Mode { c.mu.Lock(); defer c.mu.Unlock(); return c.mode }
func (c *Controller) SetMode(mode cameramode.Mode) error {
	if mode == cameramode.ModeCapture && c.Hub != nil {
		c.Hub.CloseAll()
	}
	if mode == cameramode.ModePreview && c.ProjectRunning != nil && c.ProjectRunning() {
		return ErrProjectRunning
	}
	if err := c.Manager.Switch(mode); err != nil {
		return err
	}
	c.mu.Lock()
	c.mode = mode
	c.mu.Unlock()
	return nil
}
func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mode := cameramode.Mode(strings.ToLower(req.Mode))
	if mode != cameramode.ModePreview && mode != cameramode.ModeCapture {
		http.Error(w, "mode must be preview or capture", http.StatusBadRequest)
		return
	}
	if err := c.SetMode(mode); err != nil {
		status := http.StatusServiceUnavailable
		if err == ErrProjectRunning {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"mode": string(mode)})
}
func (c *Controller) Monitor(ctxDone <-chan struct{}) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctxDone:
			return
		case <-t.C:
			if c.IdleTimeout > 0 && c.Mode() == cameramode.ModePreview && c.Hub.ClientCount() == 0 && time.Since(c.Hub.LastRequest()) >= c.IdleTimeout && (c.ProjectRunning == nil || !c.ProjectRunning()) {
				_ = c.SetMode(cameramode.ModeCapture)
			}
		}
	}
}
