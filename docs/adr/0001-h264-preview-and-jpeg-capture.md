# Use native H.264 for preview and JPEG for capture

**Status:** accepted

The camera has two mutually exclusive runtime modes: native H.264 Preview streamed as Annex-B NAL units over WebSocket, and JPEG Capture used for photos and shooting projects. A central camera manager serializes transitions; preview requests can enter Preview when no project is running, while starting a capture action immediately disconnects preview clients and waits for the first valid JPEG. A running project returns a preview-unavailable error and automatically returns from Preview to Capture after ten minutes without clients. This replaces the previous JPEG accumulation and video-generation design, which is removed entirely; ObjectBox stores project and photo metadata only, while mode and connection state remain in memory. The transition protocol is first validated on the target Raspberry Pi before integration.
