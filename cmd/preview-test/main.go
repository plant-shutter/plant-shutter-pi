package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"plant-shutter-pi/pkg/camera"
)

// 在一个 main 里完成以下测试流程：
// 1) 预览前先拍照
// 2) 启动预览并读若干帧
// 3) 预览进行时再拍照（应暂停预览帧，随后恢复）
// 4) 停止预览后再拍照
func main() {
	dev := flag.String("dev", "/dev/video0", "视频设备路径")
	pw := flag.Int("pw", 1640, "预览宽度")
	ph := flag.Int("ph", 1232, "预览高度")
	cw := flag.Int("cw", 3280, "拍照宽度")
	ch := flag.Int("ch", 2464, "拍照高度")
	n := flag.Int("n", 10, "每个阶段读取的预览帧数")
	timeout := flag.Duration("timeout", 5*time.Second, "读帧超时时间")
	flag.Parse()

	ctx := context.Background()
	cam := camera.New(ctx, *dev)
	ctrl := camera.NewController(cam)

	for iter := 1; ; iter++ {
		fmt.Printf("\n===== 循环第 %d 次 =====\n", iter)

		// 1) 预览前拍照
		fmt.Printf("[1/4] 预览前拍照: %dx%d...\n", *cw, *ch)
		img1, err := ctrl.Capture(*cw, *ch)
		if err != nil {
			fmt.Println("Capture(预览前) 失败:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(fmt.Sprintf("capture_before_%d.jpg", iter), img1, 0o644); err != nil {
			fmt.Println("保存 capture_before 失败:", err)
			os.Exit(1)
		}
		fmt.Printf("保存 capture_before_%d.jpg，大小 %d 字节\n", iter, len(img1))

		// 2) 启动预览并读若干帧
		fmt.Printf("[2/4] 启动预览: %dx%d，读取 %d 帧...\n", *pw, *ph, *n)
		prevCh, err := ctrl.StartPreview(*pw, *ph)
		if err != nil {
			fmt.Println("StartPreview 失败:", err)
			os.Exit(1)
		}
		readFrames(prevCh, *n, *timeout)

		// 3) 预览进行时拍照
		fmt.Printf("[3/4] 预览进行时拍照: %dx%d...\n", *cw, *ch)
		img2, err := ctrl.Capture(*cw, *ch)
		if err != nil {
			fmt.Println("Capture(预览中) 失败:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(fmt.Sprintf("capture_during_%d.jpg", iter), img2, 0o644); err != nil {
			fmt.Println("保存 capture_during 失败:", err)
			os.Exit(1)
		}
		fmt.Printf("保存 capture_during_%d.jpg，大小 %d 字节\n", iter, len(img2))

		// 拍照后再读取若干帧，验证预览已恢复
		fmt.Printf("拍照后继续读取 %d 帧以验证预览恢复...\n", *n)
		readFrames(prevCh, *n, *timeout)

		// 停止预览
		if err := ctrl.StopPreview(); err != nil {
			fmt.Println("StopPreview 失败:", err)
		}

		// 4) 预览后拍照
		fmt.Printf("[4/4] 预览后拍照: %dx%d...\n", *cw, *ch)
		img3, err := ctrl.Capture(*cw, *ch)
		if err != nil {
			fmt.Println("Capture(预览后) 失败:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(fmt.Sprintf("capture_after_%d.jpg", iter), img3, 0o644); err != nil {
			fmt.Println("保存 capture_after 失败:", err)
			os.Exit(1)
		}
		fmt.Printf("保存 capture_after_%d.jpg，大小 %d 字节\n", iter, len(img3))

		// 每轮之间稍作等待，避免过于频繁重配设备
		time.Sleep(500 * time.Millisecond)
	}
}

func readFrames(ch <-chan []byte, n int, timeout time.Duration) {
	got := 0
	for got < n {
		select {
		case frame, ok := <-ch:
			if !ok {
				fmt.Println("预览通道已关闭")
				os.Exit(1)
			}
			fmt.Printf("预览帧 %d，长度: %d 字节\n", got+1, len(frame))
			got++
		case <-time.After(timeout):
			fmt.Println("读取预览帧超时")
			os.Exit(1)
		}
	}
}
