package main

import (
    "bytes"
    "flag"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "sync/atomic"
    "time"

    "golang.org/x/net/websocket"
)

// Default options (can be overridden by flags)
var (
    videoPath     string
    videoDuration int // seconds
    width         int
    height        int
)

// NAL start code (Annex-B, 4-byte)
var nalStart = []byte{0x00, 0x00, 0x00, 0x01}

// splitNALAnnexB reads from r and yields raw NAL payloads (without the start code).
// It splits only on 4-byte start codes to match the reference JS implementation.
func splitNALAnnexB(r io.Reader) (<-chan []byte, <-chan error) {
    out := make(chan []byte, 32)
    errCh := make(chan error, 1)

    go func() {
        defer close(out)
        defer close(errCh)

        // Accumulate into buffer and scan for start codes incrementally.
        buf := make([]byte, 0, 64*1024)
        tmp := make([]byte, 64*1024)

        // Track where the current NAL start code begins in buf.
        haveStart := false
        startIdx := -1

        // Helper to scan buffer for start codes and emit complete NAL units.
        scan := func(final bool) error {
            // We search for start codes iteratively.
            // If we already have a start at startIdx, skip re-matching it.
            searchFrom := 0
            if haveStart && startIdx+len(nalStart) > searchFrom {
                searchFrom = startIdx + len(nalStart)
            }
            for {
                i := bytes.Index(buf[searchFrom:], nalStart)
                if i < 0 {
                    break
                }
                idx := searchFrom + i
                if !haveStart {
                    haveStart = true
                    startIdx = idx
                    searchFrom = idx + len(nalStart)
                    continue
                }

                // Found next start code; emit payload between previous start and this one.
                payload := buf[startIdx+len(nalStart) : idx]
                if len(payload) > 0 {
                    // Copy to avoid retaining large backing array.
                    cp := make([]byte, len(payload))
                    copy(cp, payload)
                    out <- cp
                }

                // Drop data before current idx to keep buffer small.
                buf = buf[idx:]
                // Reset markers relative to new buffer start
                haveStart = true
                startIdx = 0
                searchFrom = len(nalStart)
            }

            if final && haveStart {
                // Emit trailing payload until EOF.
                payload := buf[startIdx+len(nalStart):]
                if len(payload) > 0 {
                    cp := make([]byte, len(payload))
                    copy(cp, payload)
                    out <- cp
                }
            }
            return nil
        }

        for {
            n, err := r.Read(tmp)
            if n > 0 {
                buf = append(buf, tmp[:n]...)
                if e := scan(false); e != nil {
                    errCh <- e
                    return
                }
            }
            if err != nil {
                if err == io.EOF {
                    _ = scan(true)
                    return
                }
                errCh <- err
                return
            }
        }
    }()

    return out, errCh
}

// h264WS serves a WebSocket that understands REQUESTSTREAM/STOPSTREAM and
// streams NAL units from the provided H264 file with a throttled rate derived
// from file size and configured duration.
func h264WS(conn *websocket.Conn) {
    defer conn.Close()

    // Init payload for the player
    initMsg := struct {
        Action string `json:"action"`
        Width  int    `json:"width"`
        Height int    `json:"height"`
    }{Action: "init", Width: width, Height: height}

    // Send as a text frame
    if err := websocket.JSON.Send(conn, initMsg); err != nil {
        log.Printf("websocket send init failed: %v", err)
        return
    }
    // After init, switch to binary frames for H264 payloads
    conn.PayloadType = websocket.BinaryFrame

    // Open the file on demand
    f, err := os.Open(videoPath)
    if err != nil {
        log.Printf("open video failed: %v", err)
        return
    }
    defer f.Close()

    // Determine throttle rate: floor(size / duration)
    fi, err := f.Stat()
    if err != nil {
        log.Printf("stat video failed: %v", err)
        return
    }
    if videoDuration <= 0 {
        videoDuration = 1
    }
    bytesPerSec := fi.Size() / int64(videoDuration)
    if bytesPerSec <= 0 {
        bytesPerSec = 1
    }
    log.Printf("Throttle rate ~ %d kB/s", bytesPerSec/1024)

    // Create a NAL splitter over the same file handle.
    // We will read only when streaming is requested to support STOP/REQUEST semantics.
    nalCh, errCh := splitNALAnnexB(f)

    var streaming int32

    // Control: listen for REQUESTSTREAM/STOPSTREAM
    done := make(chan struct{})
    go func() {
        defer close(done)
        for {
            var msg string
            if e := websocket.Message.Receive(conn, &msg); e != nil {
                return
            }
            switch msg {
            case "REQUESTSTREAM", "REQUESTSTREAM ":
                atomic.StoreInt32(&streaming, 1)
                log.Printf("client requested stream")
            case "STOPSTREAM":
                atomic.StoreInt32(&streaming, 0)
                log.Printf("client stopped stream")
            default:
                log.Printf("unknown ws message: %q", msg)
            }
        }
    }()

    // Pace sending according to bytesPerSec.
    start := time.Now()
    sent := int64(0)

    // Consume NAL units and send when streaming is on.
    for {
        select {
        case <-done:
            return
        case e, ok := <-errCh:
            if !ok || e == nil {
                log.Printf("stream finished")
                return
            }
            log.Printf("stream error: %v", e)
            return
        case nal, ok := <-nalCh:
            if !ok {
                log.Printf("end of file reached")
                return
            }

            // If not streaming, wait until it is requested or connection closes.
            for atomic.LoadInt32(&streaming) == 0 {
                select {
                case <-done:
                    return
                case <-time.After(50 * time.Millisecond):
                }
            }

            // Prepend start code as the JS server does.
            frame := make([]byte, 0, len(nalStart)+len(nal))
            frame = append(frame, nalStart...)
            frame = append(frame, nal...)

            // Throttle: ensure average rate ~= bytesPerSec
            sent += int64(len(frame))
            desired := time.Duration(sent*int64(time.Second)) / time.Duration(bytesPerSec)
            delay := start.Add(desired).Sub(time.Now())
            if delay > 0 {
                time.Sleep(delay)
            }

            if err := websocket.Message.Send(conn, frame); err != nil {
                log.Printf("websocket send failed: %v", err)
                return
            }
        }
    }
}

func main() {
    // Defaults similar to the JS sample
    var port string
    flag.StringVar(&port, "p", ":80", "HTTP service port (e.g. :80 or :8080)")
    flag.StringVar(&videoPath, "f", "public/out.h264", "Path to H264 Annex-B file")
    flag.IntVar(&videoDuration, "t", 58, "Video duration in seconds (throttle)")
    flag.IntVar(&width, "w", 960, "Video width for init message")
    flag.IntVar(&height, "h", 540, "Video height for init message")
    flag.Parse()

    // Validate file path early
    if _, err := os.Stat(videoPath); err != nil {
        abs, _ := filepath.Abs(videoPath)
        log.Fatalf("video file not found: %s (%v)", abs, err)
    }

    // Serve static assets from repo's public folder
    fs := http.FileServer(http.Dir("public"))
    http.Handle("/", fs)

    // WebSocket endpoint compatible with http-live-player.js
    http.Handle("/stream", websocket.Handler(h264WS))
    log.Printf("Serving H264 file %q over WebSocket at %s/stream", videoPath, port)
    log.Fatal(http.ListenAndServe(port, nil))
}
