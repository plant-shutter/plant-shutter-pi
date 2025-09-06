package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"plant-shutter-pi/pkg/utils"

	"go.uber.org/zap"
)

var (
	logger *zap.SugaredLogger
)

func init() {
	logger = utils.GetLogger()
}

// Options 定义通过管道将 JPEG 帧编码为 MP4 的参数。
//
// 典型等价命令：
//
//	ffmpeg -y -hide_banner -loglevel error \
//	  -framerate 30 -f image2pipe -vcodec mjpeg -i pipe:0 \
//	  -c:v h264_v4l2m2m -b:v 12M -pix_fmt yuv420p output.mp4
type Options struct {
	// 输出文件路径（必填）
	Output string

	// 输入帧率，默认 30
	Framerate int

	// 码率，例如 "12M"；留空则不显式设置。
	Bitrate string

	// ffmpeg 可执行文件名/路径，默认 "ffmpeg"。
	FfmpegPath string

	// 额外参数（会追加到输出文件之前）。
	ExtraArgs []string
}

// Encoder 通过 stdin 管道向 ffmpeg 连续写入 JPEG 帧，生成 MP4 文件。
type Encoder struct {
	opts   Options
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr bytes.Buffer

	mu     sync.Mutex
	closed bool

	ctx    context.Context
	cancel context.CancelFunc
}

// NewEncoder 创建并启动 ffmpeg 进程，准备接收帧。
func NewEncoder(ctx context.Context, opts Options) (*Encoder, error) {
	if opts.Output == "" {
		return nil, errors.New("ffmpeg: Options.Output 不能为空")
	}
	if opts.FfmpegPath == "" {
		opts.FfmpegPath = "ffmpeg"
	}
	if opts.Framerate <= 0 {
		opts.Framerate = 30
	}

	args := []string{"-y"} // 始终覆盖，简化调用
	args = append(args,
		"-hide_banner", "-loglevel", "error",
		"-framerate", strconv.Itoa(opts.Framerate),

		// 解码与输入
		"-f", "image2pipe",
		"-vcodec", "mjpeg", // 从 stdin 读取连续 JPEG 帧
		"-i", "pipe:0",
		// 编码配置
		"-f", "matroska",
		"-c:v", "h264_v4l2m2m",
		// 输出
		"-f", "segment",
		"-segment_time", "3",
		"-reset_timestamps", "1",
	)
	if opts.Bitrate != "" {
		args = append(args, "-b:v", opts.Bitrate)
	}
	if len(opts.ExtraArgs) > 0 {
		args = append(args, opts.ExtraArgs...)
	}
	args = append(args, opts.Output)

	logger.Infof("run ffmpeg command: %s %s", opts.FfmpegPath, strings.Join(args, " "))
	cctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cctx, opts.FfmpegPath, args...)

	// 建立 stdin 管道与 stderr 捕获
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("ffmpeg: 创建 stdin 管道失败: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		return nil, fmt.Errorf("ffmpeg: 启动失败: %w", err)
	}

	enc := &Encoder{
		opts:   opts,
		cmd:    cmd,
		stdin:  stdin,
		stderr: stderr,
		ctx:    cctx,
		cancel: cancel,
	}
	return enc, nil
}

// WriteImage 写入一帧 JPEG 数据。注意：必须是完整的 JPEG 图片字节。
func (e *Encoder) WriteImage(jpeg []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("ffmpeg: encoder 已关闭")
	}
	if len(jpeg) == 0 {
		return nil
	}
	// 直接顺序写入；image2pipe+mjpeg 会逐帧解析
	if _, err := e.stdin.Write(jpeg); err != nil {
		return fmt.Errorf("ffmpeg: 写入帧失败: %w (stderr: %s)", err, e.stderr.String())
	}
	return nil
}

// Close 关闭 stdin 并等待编码进程退出。
// 若 ffmpeg 返回非零状态码，将包含 stderr 诊断信息。
func (e *Encoder) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.mu.Unlock()

	// 关闭输入，提示 ffmpeg 完结
	_ = e.stdin.Close()

	done := make(chan error, 1)
	go func() { done <- e.cmd.Wait() }()

	select {
	case err := <-done:
		e.cancel()
		if err != nil {
			return fmt.Errorf("ffmpeg: 退出错误: %w (stderr: %s)", err, e.stderr.String())
		}
		return nil
	case <-time.After(10 * time.Second):
		// 超时则取消并返回错误
		e.cancel()
		return fmt.Errorf("ffmpeg: 关闭超时 (stderr: %s)", e.stderr.String())
	}
}

// Stderr 返回累计的 ffmpeg 错误输出（用于调试）。
func (e *Encoder) Stderr() string { return e.stderr.String() }
