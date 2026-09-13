package preview

import (
	"context"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"plant-shutter-pi/pkg/cameramode"
)

type ModeManager interface {
	Switch(cameramode.Mode) error
	Frames() <-chan []byte
}

type Handler struct {
	Manager        ModeManager
	Hub            *Broadcaster
	ProjectRunning func() bool
	Upgrader       websocket.Upgrader
	Width          int
	Height         int
	pumpMu         sync.Mutex
	pumpCancel     context.CancelFunc
	pumpFrames     <-chan []byte
	pumpGeneration uint64
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.ProjectRunning != nil && h.ProjectRunning() {
		http.Error(w, "capture project is running; preview unavailable", http.StatusConflict)
		return
	}
	if err := h.Manager.Switch(cameramode.ModePreview); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	h.startPump(h.Manager.Frames())
	c, err := h.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	ch := h.Hub.Add(c)
	defer h.Hub.Remove(c)
	_ = c.WriteJSON(map[string]any{"action": "init", "width": h.Width, "height": h.Height})
	for {
		select {
		case frame, ok := <-ch:
			if !ok {
				return
			}
			if err := c.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// startPump attaches the broadcaster to the current camera frame channel.
// The camera manager replaces this channel when switching modes, so a
// sync.Once here would leave the broadcaster permanently stopped after the
// first preview session ended.
func (h *Handler) startPump(frames <-chan []byte) {
	if frames == nil {
		return
	}
	h.pumpMu.Lock()
	if h.pumpCancel != nil && h.pumpFrames == frames {
		h.pumpMu.Unlock()
		return
	}
	if h.pumpCancel != nil {
		h.pumpCancel()
	}
	pumpCtx, pumpCancel := context.WithCancel(context.Background())
	h.pumpCancel = pumpCancel
	h.pumpFrames = frames
	h.pumpGeneration++
	generation := h.pumpGeneration
	h.pumpMu.Unlock()
	go func() {
		h.Hub.Run(pumpCtx, frames)
		h.pumpMu.Lock()
		if h.pumpGeneration == generation {
			h.pumpCancel = nil
			h.pumpFrames = nil
		}
		h.pumpMu.Unlock()
	}()
}
