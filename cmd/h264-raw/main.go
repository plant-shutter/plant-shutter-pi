package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
	"golang.org/x/net/websocket"
)

var (
	frames <-chan []byte
	width  = 1920
	height = 1080
)

// h264WS 升级为 WebSocket，并将 channel 中的每帧 H264 数据以二进制消息写入
func h264WS(conn *websocket.Conn) {
	defer conn.Close()

	// 指定发送为二进制帧
	conn.PayloadType = websocket.BinaryFrame

	// 连接建立后，先发送 init 文本消息（前端用来创建画布）
	initMsg := struct {
		Action string `json:"action"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}{Action: "init", Width: width, Height: height}
	if b, err := json.Marshal(initMsg); err == nil {
		if err := websocket.Message.Send(conn, string(b)); err != nil {
			log.Printf("websocket send init failed: %v", err)
			return
		}
	}

	var (
		frame     []byte
		ok        bool
		streaming int32 // 0-stop, 1-start
	)

	// 接收控制消息：REQUESTSTREAM / STOPSTREAM
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var msg string
			if err := websocket.Message.Receive(conn, &msg); err != nil {
				return
			}
			if msg == "REQUESTSTREAM " || msg == "REQUESTSTREAM" {
				atomic.StoreInt32(&streaming, 1)
				log.Printf("client requested stream")
			} else if msg == "STOPSTREAM" {
				atomic.StoreInt32(&streaming, 0)
				log.Printf("client stopped stream")
			} else {
				log.Printf("unknown ws message: %q", msg)
			}
		}
	}()
	start := time.Now()

	for {
		select {
		case <-done:
			return
		default:
		}
		// 先取一帧
		frame, ok = <-frames
		if !ok {
			log.Printf("frame channel closed")
			return
		}

		// 统计间隔
		end := time.Now()
		log.Println(end.Sub(start))
		start = end

		if atomic.LoadInt32(&streaming) == 1 {
			// 确保前缀是 Annex-B 起始码（0x00 00 00 01）
			if len(frame) < 4 || !(frame[0] == 0 && frame[1] == 0 && frame[2] == 0 && frame[3] == 1) {
				pref := []byte{0, 0, 0, 1}
				out := make([]byte, 0, len(pref)+len(frame))
				out = append(out, pref...)
				out = append(out, frame...)
				frame = out
			}
			// 通过 WS 发送二进制消息
			if err := websocket.Message.Send(conn, frame); err != nil {
				log.Printf("websocket send failed: %v", err)
				return
			}
		}
	}
}

func main() {
	port := ":80"
	devName := "/dev/video0"
	flag.StringVar(&devName, "d", devName, "device name (path)")
	flag.StringVar(&port, "p", port, "webcam service port")

	camera, err := device.Open(
		devName,
		device.WithBufferSize(2),
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtH264, Width: uint32(width), Height: uint32(height)}),
	)

	if err != nil {
		log.Fatalf("failed to open device: %s", err)
	}
	defer camera.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		cancel()
		time.Sleep(time.Millisecond * 200)
	}()

	if err := camera.Start(ctx); err != nil {
		log.Fatalf("camera start: %s", err)
	}

	frames = camera.GetOutput()

	// 静态资源：代理 public 目录（含 index.html）
	fs := http.FileServer(http.Dir("public"))
	http.Handle("/", fs)

	// WebSocket H264 流
	log.Printf("Serving H264 over WebSocket: [%s/stream]", port)
	http.Handle("/stream", websocket.Handler(h264WS))
	log.Fatal(http.ListenAndServe(port, nil))
}
