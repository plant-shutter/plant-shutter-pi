package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
	"golang.org/x/net/websocket"
)

var (
	width      = 1920
	height     = 1080
	streamBusy int32 // 0-free, 1-in use
)

// h264WS: WebSocket 处理器，持续发送 H264 帧
func h264WS(conn *websocket.Conn) {
	defer conn.Close()

	// 指定二进制帧发送
	conn.PayloadType = websocket.BinaryFrame

	// 单连接锁：若已被占用则提示 busy 并关闭
	if !atomic.CompareAndSwapInt32(&streamBusy, 0, 1) {
		msg := struct {
			Action string `json:"action"`
			Error  string `json:"error"`
		}{Action: "error", Error: "busy"}
		if b, err := json.Marshal(msg); err == nil {
			_ = websocket.Message.Send(conn, string(b))
		}
		return
	}
	defer atomic.StoreInt32(&streamBusy, 0)

	// 连接建立后，发送初始化消息（前端可用来配置画布）
	initMsg := struct {
		Action string `json:"action"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}{Action: "init", Width: width, Height: height}
	if b, err := json.Marshal(initMsg); err == nil {
		_ = websocket.Message.Send(conn, string(b))
	}

	// 延迟到连接建立后再打开摄像头
	cam, err := device.Open(
		deviceNameConfigured,
		device.WithBufferSize(2),
		device.WithFPS(10),
		device.WithPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtH264, Width: uint32(width), Height: uint32(height)}),
	)
	if err != nil {
		log.Printf("failed to open device: %s", err)
		return
	}
	defer cam.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cam.Start(ctx); err != nil {
		log.Printf("camera start: %s", err)
		return
	}
	frames := cam.GetOutput()

	for {
		frame, ok := <-frames
		if !ok {
			log.Printf("frame channel closed")
			return
		}

		n := 8
		if len(frame) < n {
			n = len(frame)
		}
		if n > 0 {
			log.Printf("first %d bytes: % X", n, frame[:n])
		}

		if err := websocket.Message.Send(conn, frame); err != nil {
			log.Printf("websocket send failed: %v", err)
			return
		}
	}
}

func main() {
	devName := "/dev/video0"
	port := 443
	flag.StringVar(&devName, "d", devName, "device name (path)")
	flag.IntVar(&port, "p", port, "webcam service port")
	flag.Parse()

	// 保存配置的设备名，供连接时使用
	deviceNameConfigured = devName

	ips, err := getLocalIPsWithPort()
	if err != nil {
		log.Fatalf("get ips: %v", err)
		return
	}

	certPEM, keyPEM, err := SelfSignedForIPs(ips)
	if err != nil {
		log.Fatalf("gen cert failed: %v", err)
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		log.Fatalf("load key pair failed: %v", err)
	}

	// 路由
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("public"))) // 静态资源
	mux.Handle("/video", websocket.Handler(h264WS))      // WebSocket H.264 流

	// HTTPS 服务器（用内存证书）
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,           // 兼容现代浏览器
			Certificates: []tls.Certificate{tlsCert}, // 使用内存中的证书
		},
	}

	log.Printf("Serving HTTPS on %d (IPs in cert: %v); static '/', WSS at '/video'", port, ips)
	// TLSConfig 里已有证书，参数留空即可
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

// currentDeviceName 返回当前配置的设备名。
// 由于 `-d` 只在 main() 中解析，这里通过闭包方式保留。
var deviceNameConfigured string

func SelfSignedForIPs(ips []string) (certPEM, keyPEM []byte, err error) {
	// 解析 IP
	var ipSANs []net.IP
	for _, s := range ips {
		if ip := net.ParseIP(s); ip != nil {
			ipSANs = append(ipSANs, ip)
		}
	}
	if len(ipSANs) == 0 {
		return nil, nil, errors.New("no valid IP in list")
	}

	// 私钥（ECDSA P-256）
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	notBefore := time.Now().Add(-time.Hour)
	notAfter := notBefore.Add(time.Duration(365) * 24 * time.Hour)

	// 序列号
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, err
	}

	// 证书模板（只填 IP 的 SAN，不写 DNSName）
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"Plant Shutter"}},
		NotBefore:    notBefore,
		NotAfter:     notAfter,

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,

		IPAddresses: ipSANs,
		DNSNames:    []string{"localhost", "raspberry", "raspberrypi"},
	}

	// 自签
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	// 编码 PEM
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func getLocalIPsWithPort() ([]string, error) {
	var ips []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}

	return ips, nil
}
