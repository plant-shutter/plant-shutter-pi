# Plant Shutter

术语表描述相机、拍摄项目、当前画面与调参试拍之间的领域边界。

## User experience

**Current view**：用户在工作台看到的唯一主画面。没有拍摄中的项目时，它是实时预览；有拍摄中的项目时，它是该项目最新的一张照片。
_Avoid_: preview mode, capture mode

**Trial shot**：调参过程中按下“试拍”产生的一张实际拍摄分辨率 JPEG，用来验证当前参数，不属于任何拍摄项目。
_Avoid_: test photo, sample photo

## Camera

**Preview**：相机以原生 H.264 输出、供网页实时观看的运行状态。
_Avoid_: streaming mode, video mode

**Capture**：相机以 JPEG 输出、供单次拍照或延时摄影取样的运行状态。
_Avoid_: photo mode

**Camera mode**：相机当前唯一有效的 Preview 或 Capture 状态；两者互斥。

**Preview client**：当前保持实时 H.264 WebSocket 连接的网页客户端。

## Shooting

**Shooting project**：由 scheduler 管理、按时间间隔持续保存 JPEG 照片的运行项目。
_Avoid_: video project, timelapse video

**Shooting project state**：项目处于未开始、拍摄中、已暂停或已完成四种业务状态之一；暂停可以继续，完成由用户明确结束且不能继续拍摄。

**Active shooting project**：当前处于拍摄中的唯一项目。存在它时，用户必须先暂停或结束该项目，才能创建另一个项目。

**Capture sample**：一次由 Capture 状态产生并持久化的 JPEG 照片。

**Preview stream**：由单个 H.264 编码源广播给多个 Preview client 的 Annex-B NAL 单元序列。

## Failure

**Camera error**：模式切换或相机启动失败后禁止继续采集的显式状态，必须通过重试动作恢复。
