# Camera switch validation

The target Raspberry Pi was inspected over SSH on 2026-09-12. The repository's existing `cmd/h264-raw` is important evidence: it opens `/dev/video0` with `PixelFmtH264` and streams frames successfully in its intended deployment.

The camera stack exposes separate V4L2 nodes:

- A raw `v4l2-ctl` enumeration reported `/dev/video0` as YUYV, while the existing Go H.264 example explicitly configures that node as H.264. This indicates that the driver graph/configuration is dynamic and static enumeration alone is not sufficient evidence.
- `/dev/video11` is also a V4L2 H.264/MJPEG codec node and `/dev/video31` exposes JPEG on this particular Pi; these must be treated as device-specific observations, not production defaults.

The production experiment must therefore first validate the exact existing camera path: open `/dev/video0` as H.264, close it, reopen it as JPEG, and reverse the sequence. `cmd/camera-switch` accepts separate paths for unusual deployments, but defaults to one device. The harness compiles locally; the cgo V4L2 dependency cannot be cross-compiled with the host toolchain. Building on the Pi was attempted with its Go 1.15 toolchain and was stopped after the dependency build exceeded the available test window; the actual frame-switch result is still pending.

## Legacy driver run

On 2026-09-12 the Pi was rebooted with `camera_auto_detect=0` and `start_x=1`. It reported `supported=1 detected=1`, and `/dev/video0` enumerated JPEG and H264. The ARM64 harness built with `make build`, was copied to the Pi, and ran with 1920x1080 and 1280x720. Both attempts timed out waiting for the first H.264 frame. Direct `v4l2-ctl` tests also timed out for JPEG, H264, and YUYV; `libcamera-vid` reported no cameras. Reloading `bcm2835_v4l2` with `max_video_width=3280 max_video_height=2464` did not produce a frame. This is currently a camera/driver capture failure before mode switching, not evidence that the transition implementation passed.

Adding an explicit `dtoverlay=imx219` was also tested and immediately changed the firmware result to `detected=0` with `libcamera interfaces=1`; that change was reverted. The Pi is back on the prior configuration, but direct JPEG capture still times out. A physical camera connection or firmware/hardware diagnosis is required before the mode transition can be validated with real frames.

## Successful manager run

After rebooting the Pi and running single-process FFmpeg probes, JPEG and H.264 both produced valid frames. The ARM64 `camera-switch` binary was rebuilt with `make build`, copied to the Pi, and run through `cameramode.Manager` for three H.264 → JPEG cycles at 640x480. All six mode starts produced non-empty frames and the process exited successfully (`EXIT:0`).

## 100-cycle stress test

On 2026-09-12, the harness was extended with per-cycle timing and resource
sampling (`VmRSS`, `/proc/self/fd`, and goroutine count). A first 100-cycle run
revealed two leaked goroutines per cycle. The manager now gives every device a
cancellable context and closes its forwarding stream; subsequent runs kept
goroutines near four and file descriptors at seven without linear RSS growth.

The run exposed an intermittent legacy V4L2 teardown race: reopening
`/dev/video0` can fail with `device or resource busy`. The manager now retries
only this condition up to five times with 100, 200, 300, 400, and 500 ms
backoff. `device.Stop()` waits up to about 500 ms for capture shutdown, followed
by a 100 ms manager delay after close. With the bounded retry in place, all 100
cycles completed successfully: file descriptors stayed at seven, goroutines
returned to one at exit, and RSS showed no linear growth.

## Main-program mode API smoke test

The ARM64 full application and ObjectBox library were copied to the Raspberry
Pi. With an empty statics directory supplied for startup, the live service
accepted four consecutive `PUT /api/device/mode` transitions: Preview, Capture,
Preview, Capture. Every request returned HTTP 200, confirming that the
coordinator releases the legacy JPEG device before opening the native mode and
reopens JPEG capture afterward.

The same deployment created and read a project through ObjectBox, then repeated
the mode transitions. The project response contains only image and camera
metadata; the generated ObjectBox model no longer contains video-generation
properties. Legacy AVI/MP4 encoders and the multipart JPEG endpoint were removed
from the build.

The browser preview path was then tested end to end. The first WebSocket message
was the JSON init message, followed by a binary Annex-B frame beginning with
`00 00 00 01`. A capture project running at a 300 ms interval produced 13 JPEG
files in four seconds, and a Preview mode request while that project was running
returned HTTP 409 with `preview unavailable`.
