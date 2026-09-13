package preview

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Broadcaster fans out Annex-B H.264 frames to all connected clients.
type Broadcaster struct {
	mu          sync.Mutex
	clients     map[*websocket.Conn]chan []byte
	lastRequest time.Time
	headers     map[byte][]byte
	keyframe    []byte
}

func New() *Broadcaster {
	return &Broadcaster{clients: make(map[*websocket.Conn]chan []byte), headers: make(map[byte][]byte)}
}

func (b *Broadcaster) Add(c *websocket.Conn) <-chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 16)
	b.clients[c] = ch
	b.lastRequest = time.Now()
	// A client may join after the encoder has emitted its parameter sets.
	// Replay the latest SPS/PPS before subsequent slices.
	if h := b.headers[7]; h != nil {
		ch <- h
	}
	if h := b.headers[8]; h != nil {
		ch <- h
	}
	// A client can connect after the encoder's first IDR has already been
	// published. Replay the latest IDR as well; without it Broadway cannot
	// decode the following P-frames and the canvas stays black.
	if b.keyframe != nil {
		ch <- b.keyframe
	}
	return ch
}
func (b *Broadcaster) Remove(c *websocket.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.clients[c]; ok {
		delete(b.clients, c)
		close(ch)
	}
}
func (b *Broadcaster) CloseAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for c, ch := range b.clients {
		close(ch)
		_ = c.Close()
		delete(b.clients, c)
	}
}
func (b *Broadcaster) ClientCount() int       { b.mu.Lock(); defer b.mu.Unlock(); return len(b.clients) }
func (b *Broadcaster) LastRequest() time.Time { b.mu.Lock(); defer b.mu.Unlock(); return b.lastRequest }
func (b *Broadcaster) Publish(frame []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// V4L2 capture buffers may contain several Annex-B NAL units (SPS, PPS,
	// and an IDR are commonly returned together). WSAvcPlayer expects one NAL
	// per WebSocket message, so split the buffer without decoding it.
	for _, nal := range splitAnnexB(frame) {
		switch n := nalType(nal); n {
		case 7, 8:
			b.headers[n] = nal
		case 5:
			b.keyframe = nal
		}
		for _, ch := range b.clients {
			select {
			case ch <- nal:
			default:
			}
		}
	}
}

// splitAnnexB returns NAL units with a canonical four-byte start code. V4L2
// drivers may emit either three- or four-byte Annex-B prefixes; Broadway's
// protocol uses the four-byte form.
func splitAnnexB(data []byte) [][]byte {
	type marker struct{ at, size int }
	markers := make([]marker, 0, 4)
	for i := 0; i+2 < len(data); {
		if i+3 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			markers = append(markers, marker{i, 4})
			i += 4
			continue
		}
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			markers = append(markers, marker{i, 3})
			i += 3
			continue
		}
		i++
	}
	if len(markers) == 0 {
		return [][]byte{data}
	}
	result := make([][]byte, 0, len(markers))
	for i, m := range markers {
		end := len(data)
		if i+1 < len(markers) {
			end = markers[i+1].at
		}
		if end <= m.at+m.size {
			continue
		}
		nal := make([]byte, 4+end-(m.at+m.size))
		copy(nal, []byte{0, 0, 0, 1})
		copy(nal[4:], data[m.at+m.size:end])
		result = append(result, nal)
	}
	return result
}

func nalType(nal []byte) byte {
	if len(nal) < 5 {
		return 0
	}
	return nal[4] & 0x1f
}

func (b *Broadcaster) Run(ctx context.Context, frames <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-frames:
			if !ok {
				return
			}
			b.Publish(frame)
		}
	}
}
