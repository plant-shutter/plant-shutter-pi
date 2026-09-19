import { useCallback, useEffect, useRef, useState } from 'react'
import './App.css'
import './live.css'

type Config = { ID: number; name: string; value: number; minimum: number; maximum: number; step: number; default: number; isMenu?: boolean; menuItems?: Record<string, string> }
type ConfigSetter = (value: Config[] | ((current: Config[]) => Config[])) => void
type Project = { name: string; info?: string; interval: number; running?: boolean; state?: 'not_started' | 'shooting' | 'paused' | 'completed'; imageTotal?: number; latestImageName?: string; diskUsage?: string }
type ImageFile = { name: string; size: string; modTime: string }
type ImagePage = { images: ImageFile[]; total: number; totalSize: string }
type Resolution = { capture: { width: number; height: number }; preview: { width: number; height: number } }
type View = 'home' | 'projects' | 'gallery'
type IntervalMode = 'preset' | 'custom'
type IntervalUnit = 'minutes' | 'seconds'

const CONTROL = {
  autoExposure: 10094849,
  exposureTime: 10094850,
  autoWhiteBalance: 10094868,
  redBalance: 9963790,
  blueBalance: 9963791,
  autoIso: 10094872,
  iso: 10094871,
} as const

const fallback: Config[] = [
  { ID: 9963776, name: 'Brightness', value: 0, minimum: -64, maximum: 64, step: 1, default: 0 },
  { ID: 9963777, name: 'Contrast', value: 32, minimum: 0, maximum: 64, step: 1, default: 32 },
  { ID: 9963778, name: 'Saturation', value: 32, minimum: 0, maximum: 64, step: 1, default: 32 },
  { ID: 9963803, name: 'Sharpness', value: 1, minimum: 0, maximum: 6, step: 1, default: 1 },
]
const labels: Record<string, string> = {
  Brightness: '亮度', Contrast: '对比度', Saturation: '饱和度', Sharpness: '锐度',
  'Exposure Time, Absolute': '曝光时间', 'ISO Sensitivity': 'ISO 感光度',
  'Auto Exposure': '自动曝光', 'White Balance, Auto & Preset': '白平衡',
  'ISO Sensitivity, Auto': '自动 ISO', 'Red Balance': '红色增益', 'Blue Balance': '蓝色增益',
  'Color Effects, CbCr': '色彩效果底层参数', 'Compression Quality': '压缩质量',
}

function menuValueLabel(config: Config) {
  return config.menuItems?.[String(config.value)]?.toLowerCase() || ''
}

function isAutoEnabled(config: Config) {
  const menuLabel = menuValueLabel(config)
  if (menuLabel) return menuLabel.includes('auto')
  if (config.ID === CONTROL.autoExposure) return config.value === 0
  if (config.ID === CONTROL.autoWhiteBalance) return config.value !== 0
  if (config.ID === CONTROL.autoIso) return config.value === 1
  return false
}

function dependencyFor(config: Config) {
  if (config.ID === CONTROL.exposureTime) return CONTROL.autoExposure
  if (config.ID === CONTROL.redBalance || config.ID === CONTROL.blueBalance) return CONTROL.autoWhiteBalance
  if (config.ID === CONTROL.iso) return CONTROL.autoIso
  return undefined
}

const CONTROL_ORDER = [
  CONTROL.autoExposure, CONTROL.exposureTime,
  CONTROL.autoWhiteBalance, CONTROL.redBalance, CONTROL.blueBalance,
  CONTROL.autoIso, CONTROL.iso,
]

// V4L2 exposes exposure_time_absolute in units of 100 microseconds.
// The H.264 preview path is not reliable once exposure exceeds 100 ms.
const EXPOSURE_PREVIEW_LIMIT = 1000
const EXPOSURE_WARNING_STORAGE_KEY = 'plant-shutter.preview-exposure-warning.v1'

function formatExposure(value: number) {
  const milliseconds = value / 10
  return `${Number.isInteger(milliseconds) ? milliseconds.toFixed(0) : milliseconds.toFixed(1)}ms`
}

function exposureWarningWasSeen() {
  if (typeof window === 'undefined') return false
  try { return window.localStorage.getItem(EXPOSURE_WARNING_STORAGE_KEY) === '1' } catch { return false }
}

function rememberExposureWarning() {
  if (typeof window === 'undefined') return
  try { window.localStorage.setItem(EXPOSURE_WARNING_STORAGE_KEY, '1') } catch { /* Private browsing may reject storage. */ }
}

function sortControls(configs: Config[]) {
  const order = new Map<number, number>(CONTROL_ORDER.map((id, index) => [id, index]))
  return [...configs].sort((a, b) => (order.get(a.ID) ?? CONTROL_ORDER.length) - (order.get(b.ID) ?? CONTROL_ORDER.length))
}

const QUALITY_PRESETS = [
  { value: 90, label: '高画质', detail: '细节更多，文件较大' },
  { value: 60, label: '平衡', detail: '画质和空间占用平衡' },
  { value: 30, label: '节省空间', detail: '文件更小，细节较少' },
]

function isShooting(project: Project) {
  return project.state === 'shooting' || project.running === true
}

function projectStateLabel(project: Project) {
  if (isShooting(project)) return '拍摄中'
  if (project.state === 'completed') return '已完成'
  if (project.state === 'paused') return '已暂停'
  return '未开始'
}

function isAdvancedControl(config: Config) {
  return config.ID === 10291459 || config.ID === 9963818
}

function nearestQualityPreset(value: number) {
  return QUALITY_PRESETS.reduce((nearest, preset) => Math.abs(preset.value - value) < Math.abs(nearest.value - value) ? preset : nearest)
}

function formatInterval(interval: number) {
  if (interval < 60000) return `每 ${Math.max(1, Math.round(interval / 1000))} 秒`
  if (interval % 3600000 === 0) return `每 ${interval / 3600000} 小时`
  return `每 ${Math.round(interval / 60000)} 分钟`
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...init })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(body?.message || body?.error || `请求失败（${response.status}）`)
  return (body?.data ?? body) as T
}

declare global { interface Window { WSAvcPlayer?: new (canvas: HTMLCanvasElement, renderMode?: string, latency?: number, debug?: number) => { connect: (url: string) => void; disconnect: () => void; playStream?: () => void } } }

function Preview({ online, resolution, compact = false }: { online: boolean; resolution: Resolution['preview']; compact?: boolean }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [connected, setConnected] = useState(false)
  useEffect(() => {
    if (!online || !window.WSAvcPlayer || !canvasRef.current) return
    const player = new window.WSAvcPlayer(canvasRef.current, 'webgl', 1, 0)
    const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/device/preview`
    player.connect(url)
    setConnected(true)
    return () => { player.disconnect(); setConnected(false) }
  }, [online])
  return <div className={`preview-stage ${compact ? 'compact' : ''}`} aria-label={online ? '相机预览' : '相机离线'}><div className="grid" /><canvas ref={canvasRef} className={connected ? 'live-canvas visible' : 'live-canvas'} /><div className="plant" aria-hidden="true"><i className="stem" /><i className="leaf a" /><i className="leaf b" /><i className="leaf c" /><i className="pot" /></div><div className="preview-label"><b />{connected ? 'LIVE · H.264' : 'LIVE · PREVIEW'} <span>{resolution.width} × {resolution.height}</span></div>{!online && <div className="offline">等待相机信号<small>连接设备后将自动显示画面</small></div>}{online && !connected && <div className="offline">正在连接预览<small>设备在线，等待第一帧画面</small></div>}</div>
}

function LatestPhoto({ project, compact = false }: { project: Project; compact?: boolean }) {
  const [failed, setFailed] = useState(false)
  const [retry, setRetry] = useState(0)
  const source = `/api/project/${encodeURIComponent(project.name)}/image/latest?image=${encodeURIComponent(project.latestImageName || String(project.imageTotal || 0))}&retry=${retry}`
  useEffect(() => {
    setFailed(false)
    setRetry(0)
  }, [project.name, project.latestImageName, project.imageTotal])
  useEffect(() => {
    if (!failed) return
    const timer = window.setTimeout(() => {
      setFailed(false)
      setRetry(value => value + 1)
    }, 5000)
    return () => window.clearTimeout(timer)
  }, [failed])
  return <div className={`preview-stage photo-stage ${compact ? 'compact' : ''}`} aria-label={`${project.name} 最新照片`}>
    {project.imageTotal && project.imageTotal > 0 && !failed ? <img src={source} alt={`${project.name} 最新照片`} onError={() => setFailed(true)} /> : <div className="offline">等待第一次采集<small>项目开始后，最新照片会显示在这里</small></div>}
    <div className="preview-label"><b className="photo-dot" />最新照片 <span>{project.name}</span></div>
  </div>
}

function App() {
  const [view, setView] = useState<View>('home')
  const [selected, setSelected] = useState<Project | null>(null)
  const [projects, setProjects] = useState<Project[]>([])
  const [online, setOnline] = useState(false)
  const [configs, setConfigs] = useState<Config[]>(fallback)
  const [draft, setDraft] = useState<Config[]>(fallback)
  const [images, setImages] = useState<ImagePage | null>(null)
  const [step, setStep] = useState(0)
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [name, setName] = useState('')
  const [info, setInfo] = useState('')
  const [interval, setIntervalValue] = useState(300000)
  const [intervalMode, setIntervalMode] = useState<IntervalMode>('preset')
  const [customIntervalValue, setCustomIntervalValue] = useState('1')
  const [customIntervalUnit, setCustomIntervalUnit] = useState<IntervalUnit>('minutes')
  const [days, setDays] = useState(1)
  const [resolution, setResolution] = useState<Resolution>({ capture: { width: 0, height: 0 }, preview: { width: 0, height: 0 } })
  const [trialUrl, setTrialUrl] = useState<string | null>(null)
  const [trialStreamUrl, setTrialStreamUrl] = useState<string | null>(null)
  const [trialStale, setTrialStale] = useState(false)
  const [trialBusy, setTrialBusy] = useState(false)

  const refresh = useCallback(async () => {
    const [list, camera, dimensions] = await Promise.allSettled([api<Project[]>('/api/project'), api<{ available: boolean }>('/api/device/camera'), api<Resolution>('/api/device/resolution')])
    if (list.status === 'fulfilled') setProjects(list.value || [])
    if (camera.status === 'fulfilled') setOnline(camera.value.available)
    if (dimensions.status === 'fulfilled') setResolution(dimensions.value)
  }, [])
  useEffect(() => { refresh().catch(() => setNotice('设备暂不可用，请检查服务连接')) }, [refresh])

  const running = projects.find(isShooting)
  const runningName = running?.name
  useEffect(() => {
    if (!runningName) return
    const timer = window.setInterval(() => { refresh().catch(() => undefined) }, 5000)
    return () => window.clearInterval(timer)
  }, [refresh, runningName])

  const openProject = async (project: Project) => { setSelected(project); setView('projects'); try { setImages(await api<ImagePage>(`/api/project/${encodeURIComponent(project.name)}/image?page=1&page_size=60`)) } catch { setNotice('项目照片暂时无法加载') } }
  const beginCreate = async () => {
    if (running) {
      setSelected(running)
      setView('projects')
      setNotice(`项目“${running.name}”正在拍摄，请先暂停或结束后再创建新项目`)
      return
    }
    setBusy(true)
    setStep(1)
    setIntervalMode('preset')
    setIntervalValue(300000)
    setCustomIntervalValue('1')
    setCustomIntervalUnit('minutes')
    try {
      const controls = await api<Config[]>('/api/device/config')
      if (controls.length) { setConfigs(controls); setDraft(controls) }
      setTrialUrl(null)
      setTrialStreamUrl(null)
      setTrialStale(false)
    } catch { setStep(0); setNotice('相机暂时无法进入调参界面，请稍后重试') } finally { setBusy(false) }
  }
  const apply = async (config: Config, value: number) => {
    setDraft(current => current.map(item => item.ID === config.ID ? { ...item, value } : item))
    setSaved(false)
    if (trialUrl) setTrialStale(true)
    try {
      await api('/api/device/config', { method: 'PUT', body: JSON.stringify([{ ID: config.ID, Value: value }]) })
      setNotice(`${labels[config.name] || config.name} 已应用`)
    } catch { setNotice('参数已更新，设备连接后会同步') }
  }
  const updateProject = async (project: Project, nextRunning: boolean) => { setBusy(true); try { await api('/api/project', { method: 'PUT', body: JSON.stringify({ name: project.name, running: nextRunning }) }); await refresh(); setSelected({ ...project, running: nextRunning, state: nextRunning ? 'shooting' : 'paused' }); setNotice(nextRunning ? '拍摄已继续' : '拍摄已暂停，实时画面已恢复') } catch (error) { setNotice(error instanceof Error ? error.message : '项目状态更新失败') } finally { setBusy(false) } }
  const endProject = async (project: Project) => { if (!window.confirm(`结束项目“${project.name}”？结束后不能继续拍摄，只能查看或重置。`)) return; setBusy(true); try { await api('/api/project', { method: 'PUT', body: JSON.stringify({ name: project.name, completed: true }) }); await refresh(); setSelected({ ...project, running: false, state: 'completed' }); setNotice('项目已结束，实时画面已恢复') } catch (error) { setNotice(error instanceof Error ? error.message : '项目结束失败') } finally { setBusy(false) } }
  const loadImages = async (project: Project) => { try { setImages(await api<ImagePage>(`/api/project/${encodeURIComponent(project.name)}/image?page=1&page_size=60`)) } catch { setNotice('项目照片暂时无法加载') } }
  const openGallery = async () => { if (!selected) return; await loadImages(selected); setView('gallery') }
  const deleteImage = async (image: ImageFile) => { if (!selected || !window.confirm(`删除照片“${image.name}”？`)) return; try { await api(`/api/project/${encodeURIComponent(selected.name)}/image/${encodeURIComponent(image.name)}`, { method: 'DELETE' }); setImages(images ? { ...images, images: images.images.filter(item => item.name !== image.name), total: images.total - 1 } : null); setNotice('照片已删除') } catch { setNotice('照片删除失败，请重试') } }
  const clearImages = async () => { if (!selected || !window.confirm('清空这个项目的全部照片？此操作无法撤销。')) return; try { await api(`/api/project/${encodeURIComponent(selected.name)}/image`, { method: 'DELETE' }); setImages({ images: [], total: 0, totalSize: '0 B' }); setNotice('照片已清空') } catch { setNotice('照片清理失败，请重试') } }
  const trialShot = async () => {
    setTrialBusy(true)
    try {
      await api('/api/device/config', { method: 'PUT', body: JSON.stringify(draft.map(config => ({ ID: config.ID, Value: config.value }))) })
      // Let the live player unmount and release its websocket before the
      // backend opens the JPEG source for the trial shot.
      await new Promise(resolve => window.setTimeout(resolve, 200))
      const response = await fetch('/api/device/trial-shot', { method: 'POST' })
      if (!response.ok) throw new Error('试拍失败，请检查相机连接')
      const blob = await response.blob()
      if (trialUrl) URL.revokeObjectURL(trialUrl)
      setTrialUrl(URL.createObjectURL(blob))
      setTrialStale(false)
      setNotice('试拍完成，已显示实际拍摄分辨率原图')
    } catch (error) { setNotice(error instanceof Error ? error.message : '试拍失败，请重试') } finally { setTrialBusy(false) }
  }
  const startTrialStream = async () => {
    setTrialBusy(true)
    try {
      await api('/api/device/config', { method: 'PUT', body: JSON.stringify(draft.map(config => ({ ID: config.ID, Value: config.value }))) })
      await new Promise(resolve => window.setTimeout(resolve, 200))
      if (trialUrl) URL.revokeObjectURL(trialUrl)
      setTrialUrl(null)
      setTrialStale(false)
      setTrialStreamUrl(`/api/device/trial-stream?session=${Date.now()}`)
      setNotice('连续试拍已开始，画面会持续更新')
    } catch (error) { setNotice(error instanceof Error ? error.message : '连续试拍失败，请重试') } finally { setTrialBusy(false) }
  }
  const stopTrialStream = () => {
    setTrialStreamUrl(null)
    setNotice('连续试拍已停止')
  }
  const returnToLive = () => { if (trialUrl) URL.revokeObjectURL(trialUrl); setTrialUrl(null); setTrialStreamUrl(null); setTrialStale(false) }
  const closeCreate = async () => { returnToLive(); await api('/api/device/mode', { method: 'PUT', body: JSON.stringify({ mode: 'preview' }) }).catch(() => undefined); setStep(0) }
  const create = async () => {
    if (!name.trim()) { setNotice('请填写项目名称'); setStep(1); return }
    if (!saved) { setNotice('请先保存相机参数'); setStep(2); return }
    setBusy(true)
    let created = false
    try {
      const camera = Object.fromEntries(draft.map(config => [String(config.ID), config.value]))
      await api('/api/project', { method: 'POST', body: JSON.stringify({ name, info, interval, camera }) })
      created = true
      returnToLive()
      await refresh()
      setStep(0)
      setNotice('项目已创建，已自动开始拍摄')
      setView('projects')
    } catch (error) {
      setNotice(created ? '项目已创建，但界面刷新失败，请重新打开项目' : error instanceof Error ? error.message : '项目创建失败')
      if (created) {
        await refresh().catch(() => undefined)
        setStep(0)
        setView('projects')
      }
    } finally { setBusy(false) }
  }
  useEffect(() => () => { if (trialUrl) URL.revokeObjectURL(trialUrl) }, [trialUrl])
  const estimate = Math.round(days * 86400000 / interval)
  const intervalLabel = intervalMode === 'custom' ? `每 ${customIntervalValue || '1'} ${customIntervalUnit === 'minutes' ? '分钟' : '秒'}` : interval >= 3600000 ? `每 ${interval / 3600000} 小时` : `每 ${interval / 60000} 分钟`
  const updateCustomInterval = (value: string, unit = customIntervalUnit) => {
    const digits = value.replace(/[^0-9]/g, '')
    setCustomIntervalValue(digits)
    const numeric = Number(digits)
    if (numeric > 0) setIntervalValue(numeric * (unit === 'minutes' ? 60000 : 1000))
  }
  const selectInterval = (value: string) => {
    if (value === 'custom') {
      setIntervalMode('custom')
      const numeric = Number(customIntervalValue) || 1
      setIntervalValue(numeric * (customIntervalUnit === 'minutes' ? 60000 : 1000))
      return
    }
    setIntervalMode('preset')
    setIntervalValue(Number(value))
  }

  return <div className={`shell ${step > 0 ? 'create-open' : ''}`}><main><header><div><p>CONTROL ROOM / LOCAL</p><h1>{view === 'home' ? '植物生长监控' : view === 'gallery' ? '照片图库' : selected ? selected.name : '拍摄项目'}</h1></div>{view !== 'home' && <button className="header-back secondary" onClick={() => setView('home')}>← 返回总览</button>}<span className={online ? 'pill ok' : 'pill warn'}>● {online ? '设备在线' : '设备离线'}</span></header>{notice && <div className="notice" role="status">{notice}<button aria-label="关闭提示" onClick={() => setNotice('')}>×</button></div>}{view === 'home' && step === 0 && <Home online={online} resolution={resolution} busy={busy} running={running} projects={projects} onCreate={beginCreate} onProject={openProject} />}{view === 'projects' && step === 0 && <Projects projects={projects} selected={selected} busy={busy} images={images} onSelect={openProject} onCreate={beginCreate} onRun={updateProject} onEnd={endProject} onGallery={openGallery} onClear={clearImages} />}{view === 'gallery' && step === 0 && <Gallery project={selected} images={images} onDelete={deleteImage} onClear={clearImages} />}</main>{step > 0 && <Wizard step={step} setStep={setStep} onClose={closeCreate} online={online} resolution={resolution} draft={draft} configs={configs} saved={saved} setSaved={setSaved} setDraft={setDraft} apply={apply} trialUrl={trialUrl} trialStreamUrl={trialStreamUrl} trialStale={trialStale} trialBusy={trialBusy} onTrialShot={trialShot} onStartTrialStream={startTrialStream} onStopTrialStream={stopTrialStream} onReturnToLive={returnToLive} name={name} setName={setName} info={info} setInfo={setInfo} interval={interval} intervalMode={intervalMode} customIntervalValue={customIntervalValue} customIntervalUnit={customIntervalUnit} updateCustomInterval={updateCustomInterval} setCustomIntervalUnit={setCustomIntervalUnit} selectInterval={selectInterval} days={days} setDays={setDays} estimate={estimate} intervalLabel={intervalLabel} busy={busy} onCreate={create} />}</div>
}

function Home({ online, resolution, busy, running, projects, onCreate, onProject }: { online: boolean; resolution: Resolution; busy: boolean; running?: Project; projects: Project[]; onCreate: () => void; onProject: (project: Project) => void }) {
  const currentProject = running
  return <><section className="dashboard"><div className="card preview-card"><div className="heading"><div><p>{currentProject ? 'CURRENT PROJECT' : 'CURRENT VIEW'}</p><h2>{currentProject ? currentProject.name : '实时画面'}</h2></div><span className={`mode ${currentProject ? 'project-mode' : ''}`}>{currentProject ? '拍摄中' : '实时预览'}</span></div>{currentProject ? <LatestPhoto project={currentProject} /> : <Preview online={online} resolution={resolution.preview} />}<div className="preview-actions"><small>● {currentProject ? '每 5 秒刷新最新照片' : online ? '连接稳定' : '等待相机信号'}</small>{currentProject && <button className="secondary" disabled={busy} onClick={() => onProject(currentProject)}>查看项目</button>}</div></div><div className="card overview"><div className="heading"><div><p>AT A GLANCE</p><h2>设备概况</h2></div></div><div className="metric">◉<span>当前状态<strong>{currentProject ? '正在拍摄' : '空闲，可创建项目'}</strong></span></div><div className="metric">◷<span>当前项目<strong>{currentProject?.name || '尚未开始'}</strong></span></div><div className="metric">▧<span>最近采集<strong>{currentProject?.imageTotal || projects[0]?.imageTotal || 0} <small>张照片</small></strong></span></div><button className="primary full" onClick={onCreate}>＋ 创建拍摄项目</button></div></section><section className="lower"><div className="card list"><div className="heading"><div><p>SHOOTING PROJECTS</p><h2>拍摄项目</h2></div></div>{projects.length ? projects.slice(0, 3).map(project => <button className="row" key={project.name} onClick={() => onProject(project)}><span>{project.name[0]}</span><div><strong>{project.name}</strong><small>{projectStateLabel(project)} · {project.imageTotal || 0} 张照片</small></div><i className="pill">{projectStateLabel(project)}</i></button>) : <div className="empty">◫<p>还没有拍摄项目</p><small>创建一个项目开始记录植物的生长变化</small></div>}</div><div className="card list"><div className="heading"><div><p>RECENT CAPTURE</p><h2>最近照片</h2></div></div><div className="empty">◌<p>完成第一次拍摄后<br />照片会出现在这里</p></div></div></section></>
}

function Projects({ projects, selected, busy, images, onSelect, onCreate, onRun, onEnd, onGallery, onClear }: { projects: Project[]; selected: Project | null; busy: boolean; images: ImagePage | null; onSelect: (project: Project) => void; onCreate: () => void; onRun: (project: Project, running: boolean) => void; onEnd: (project: Project) => void; onGallery: () => void; onClear: () => void }) { return <div className="project-layout"><section className="card project-catalog"><div className="heading"><div><p>SHOOTING PROJECTS</p><h2>你的项目</h2></div><button className="primary" onClick={onCreate}>＋ 新项目</button></div>{projects.length ? projects.map(project => <button className={`project-select ${selected?.name === project.name ? 'selected' : ''}`} key={project.name} onClick={() => onSelect(project)}><span>{project.name[0]}</span><div><strong>{project.name}</strong><small>{projectStateLabel(project)} · {project.imageTotal || 0} 张照片</small></div><b>›</b></button>) : <div className="empty">还没有项目</div>}</section>{selected ? <section className="card project-detail"><div className="detail-head"><div><p>PROJECT DETAIL</p><h2>{selected.name}</h2><small>{selected.info || '没有项目说明'}</small></div><span className={`pill ${isShooting(selected) ? 'ok' : ''}`}>{projectStateLabel(selected)}</span></div><div className="detail-stats"><span><small>拍摄间隔</small><strong>{formatInterval(selected.interval)}</strong></span><span><small>已采集</small><strong>{selected.imageTotal || 0} 张</strong></span><span><small>占用空间</small><strong>{selected.diskUsage || '—'}</strong></span></div><div className="detail-actions">{selected.state !== 'completed' && <button className="primary" disabled={busy} onClick={() => onRun(selected, !isShooting(selected))}>{isShooting(selected) ? '暂停拍摄' : '继续拍摄'} <span>→</span></button>}{isShooting(selected) || selected.state === 'paused' ? <button className="danger" disabled={busy} onClick={() => onEnd(selected)}>结束项目</button> : null}<button className="secondary" onClick={onGallery}>打开照片库</button></div><div className="detail-photos"><div className="section-title"><strong>最近照片</strong><button onClick={onGallery}>查看全部 →</button></div>{images?.images?.length ? <div className="thumb-grid">{images.images.slice(-4).reverse().map(image => <img key={image.name} src={`/api/project/${encodeURIComponent(selected.name)}/image/${encodeURIComponent(image.name)}`} alt={`${selected.name} ${image.name}`} loading="lazy" />)}</div> : <div className="empty small">还没有照片<br /><small>开始拍摄后，照片会出现在这里</small></div>}<div className="danger-zone"><button className="danger" onClick={onClear}>清空全部照片</button></div></div></section> : <div className="card empty detail-empty">选择一个项目查看详情</div>}</div> }

function Gallery({ project, images, onDelete, onClear }: { project: Project | null; images: ImagePage | null; onDelete: (image: ImageFile) => void; onClear: () => void }) { if (!project) return <div className="card empty">请先从拍摄项目中选择一个项目</div>; return <section className="card gallery"><div className="gallery-head"><div><p>PHOTO LIBRARY</p><h2>{project.name}</h2><small>{images?.total || 0} 张照片 · {images?.totalSize || '—'}</small></div><button className="danger" onClick={onClear}>清空图库</button></div>{images?.images?.length ? <div className="gallery-grid">{images.images.map(image => <figure key={image.name}><img src={`/api/project/${encodeURIComponent(project.name)}/image/${encodeURIComponent(image.name)}`} alt={image.name} loading="lazy" /><figcaption><span>{new Date(image.modTime).toLocaleString('zh-CN')}</span><button aria-label={`删除 ${image.name}`} onClick={() => onDelete(image)}>删除</button></figcaption></figure>)}</div> : <div className="empty">◌<p>还没有照片</p><small>开始拍摄后，照片会出现在这里</small></div>}</section> }

function CameraControl({ config, draft, setDraft, apply, previewing = false, onPreviewLimit }: { config: Config; draft: Config[]; setDraft: ConfigSetter; apply: (config: Config, value: number) => void; previewing?: boolean; onPreviewLimit?: () => void }) {
  const valueRef = useRef(config.value)
  const committedValueRef = useRef(config.value)
  const dirtyRef = useRef(false)
  const holdTimeoutRef = useRef<number | null>(null)
  const holdIntervalRef = useRef<number | null>(null)
  useEffect(() => {
    // A range change updates the draft before the pointer is released. Keep
    // the edited value until commitValue sends it to the backend.
    if (!dirtyRef.current) {
      valueRef.current = config.value
      committedValueRef.current = config.value
    }
  }, [config.value])
  useEffect(() => () => {
    if (holdTimeoutRef.current !== null) window.clearTimeout(holdTimeoutRef.current)
    if (holdIntervalRef.current !== null) window.clearInterval(holdIntervalRef.current)
  }, [])

  const dependency = dependencyFor(config)
  const parent = dependency ? draft.find(item => item.ID === dependency) : undefined
  if (parent && isAutoEnabled(parent)) return null

  const title = labels[config.name] || config.name
  const inputId = `camera-control-${config.ID}`
  const isAutomatic = config.ID === CONTROL.autoExposure || config.ID === CONTROL.autoWhiteBalance || config.ID === CONTROL.autoIso
  const isCompression = config.ID === 10291459
  const isExposure = config.ID === CONTROL.exposureTime
  const quality = isCompression ? nearestQualityPreset(config.value) : undefined

  const clearHold = () => {
    if (holdTimeoutRef.current !== null) { window.clearTimeout(holdTimeoutRef.current); holdTimeoutRef.current = null }
    if (holdIntervalRef.current !== null) { window.clearInterval(holdIntervalRef.current); holdIntervalRef.current = null }
  }
  const updateValue = (value: number) => {
    const next = Math.min(config.maximum, Math.max(config.minimum, value))
    valueRef.current = next
    dirtyRef.current = true
    setDraft(current => current.map(item => item.ID === config.ID ? { ...item, value: next } : item))
    if (previewing && isExposure && next > EXPOSURE_PREVIEW_LIMIT) onPreviewLimit?.()
  }
  const nudge = (direction: number) => updateValue(valueRef.current + direction * (config.step || 1))
  const commitValue = () => {
    clearHold()
    const value = valueRef.current
    if (value === committedValueRef.current) {
      dirtyRef.current = false
      return
    }
    committedValueRef.current = value
    dirtyRef.current = false
    apply(config, value)
  }
  const applyDirectValue = (value: number) => {
    valueRef.current = value
    committedValueRef.current = value
    dirtyRef.current = false
    apply(config, value)
  }
  const startHold = (direction: number) => {
    clearHold()
    nudge(direction)
    holdTimeoutRef.current = window.setTimeout(() => {
      holdIntervalRef.current = window.setInterval(() => nudge(direction), 85)
    }, 350)
  }
  const handleStepKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>, direction: number) => {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    if (!event.repeat) startHold(direction)
  }
  const handleStepKeyUp = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); commitValue() }
  }
  const handleRangeChange = (event: React.ChangeEvent<HTMLInputElement>) => updateValue(Number(event.target.value))

  return <div className={`control ${dependency ? 'dependent' : ''}`}>
    <div className="control-head">
      <label htmlFor={inputId}>
        <strong>{title}</strong>
        {isAutomatic && <small>{isAutoEnabled(config) ? '自动调整中' : '手动模式'}</small>}
      </label>
      <span>
        <output>{isCompression ? quality?.label : config.isMenu ? (config.menuItems?.[String(config.value)] || config.value) : isExposure ? `${config.value} (${formatExposure(config.value)})` : config.value}</output>
        <button type="button" className="control-reset" onClick={() => applyDirectValue(config.default)} aria-label={`将${title}恢复默认`}>恢复默认</button>
      </span>
    </div>
    {isCompression ? <select id={inputId} value={quality?.value} onChange={event => apply(config, Number(event.target.value))}>{QUALITY_PRESETS.map(preset => <option key={preset.value} value={preset.value}>{preset.label}（{preset.value}）</option>)}</select> : config.isMenu ? <select id={inputId} value={config.value} onChange={event => apply(config, Number(event.target.value))}>{Object.entries(config.menuItems || {}).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select> : <div className="range-control"><input id={inputId} aria-label={title} type="range" min={config.minimum} max={config.maximum} step={config.step || 1} value={config.value} onChange={handleRangeChange} onPointerUp={commitValue} onPointerCancel={commitValue} onKeyUp={event => { if (event.key === 'ArrowLeft' || event.key === 'ArrowRight' || event.key === 'Home' || event.key === 'End') commitValue() }} /><span className="range-stepper" aria-label={`${title}微调`}><button type="button" aria-label={`${title}增加`} title="增加" onPointerDown={event => { event.currentTarget.setPointerCapture(event.pointerId); startHold(1) }} onPointerUp={commitValue} onPointerCancel={commitValue} onKeyDown={event => handleStepKeyDown(event, 1)} onKeyUp={handleStepKeyUp}>▲</button><button type="button" aria-label={`${title}减少`} title="减少" onPointerDown={event => { event.currentTarget.setPointerCapture(event.pointerId); startHold(-1) }} onPointerUp={commitValue} onPointerCancel={commitValue} onKeyDown={event => handleStepKeyDown(event, -1)} onKeyUp={handleStepKeyUp}>▼</button></span></div>}
    {isCompression && <small className="control-detail">{quality?.detail}</small>}
  </div>
}

function Wizard({ step, setStep, onClose, online, resolution, draft, configs, saved, setSaved, setDraft, apply, trialUrl, trialStreamUrl, trialStale, trialBusy, onTrialShot, onStartTrialStream, onStopTrialStream, onReturnToLive, name, setName, info, setInfo, interval, intervalMode, customIntervalValue, customIntervalUnit, updateCustomInterval, setCustomIntervalUnit, selectInterval, days, setDays, estimate, intervalLabel, busy, onCreate }: {
  step: number
  setStep: (step: number) => void
  onClose: () => void
  online: boolean
  resolution: Resolution
  draft: Config[]
  configs: Config[]
  saved: boolean
  setSaved: (value: boolean) => void
  setDraft: ConfigSetter
  apply: (config: Config, value: number) => void
  trialUrl: string | null
  trialStreamUrl: string | null
  trialStale: boolean
  trialBusy: boolean
  onTrialShot: () => void
  onStartTrialStream: () => void
  onStopTrialStream: () => void
  onReturnToLive: () => void
  name: string
  setName: (value: string) => void
  info: string
  setInfo: (value: string) => void
  interval: number
  intervalMode: IntervalMode
  customIntervalValue: string
  customIntervalUnit: IntervalUnit
  updateCustomInterval: (value: string, unit?: IntervalUnit) => void
  setCustomIntervalUnit: (value: IntervalUnit) => void
  selectInterval: (value: string) => void
  days: number
  setDays: (value: number) => void
  estimate: number
  intervalLabel: string
  busy: boolean
  onCreate: () => void
}) {
  const exposure = draft.find(config => config.ID === CONTROL.exposureTime)
  const previewBlocked = step === 2 && !trialUrl && !trialStreamUrl && exposure !== undefined && exposure.value > EXPOSURE_PREVIEW_LIMIT
  const [exposureWarning, setExposureWarning] = useState(false)
  const warningSeen = useRef(exposureWarningWasSeen())
  const wasPreviewBlocked = useRef(false)
  const showExposureWarning = () => {
    if (warningSeen.current) return
    warningSeen.current = true
    rememberExposureWarning()
    setExposureWarning(true)
  }
  useEffect(() => {
    if (previewBlocked && !wasPreviewBlocked.current) showExposureWarning()
    if (!previewBlocked) setExposureWarning(false)
    wasPreviewBlocked.current = previewBlocked
  }, [previewBlocked])

  return <div className="create-page" aria-labelledby="wizard-title"><section className="wizard"><header><div><p>NEW SHOOTING PROJECT</p><h2 id="wizard-title">创建拍摄项目</h2></div><button aria-label="关闭" onClick={onClose}>×</button></header><div className="steps">{['项目信息', '相机调试', '拍摄配置', '确认创建'].map((label, index) => <span className={step === index + 1 ? 'current' : step > index + 1 ? 'done' : ''} key={label}><b>{step > index + 1 ? '✓' : index + 1}</b>{label}</span>)}</div><div className="wizard-body">
    {step === 1 && <div className="intro"><div><p>STEP 01 / 04</p><h3>先定义这次拍摄</h3><label htmlFor="project-name">项目名称<input id="project-name" autoFocus value={name} onChange={event => setName(event.target.value)} placeholder="例如：龟背竹 · 春季生长" /></label><label htmlFor="project-info">项目说明 <small>可选</small><textarea id="project-info" value={info} onChange={event => setInfo(event.target.value)} placeholder="记录拍摄地点、植物品种或实验备注" /></label></div></div>}
    {step === 2 && <div className="tune"><div><div className={`trial-stage ${previewBlocked ? 'preview-blocked' : ''}`}>{trialUrl ? <><img src={trialUrl} alt="试拍原图" /><span className="trial-badge">试拍原图 · {resolution.capture.width} × {resolution.capture.height}</span>{trialStale && <span className="trial-stale">参数已变化，请重新试拍</span>}</> : trialStreamUrl ? <><img src={trialStreamUrl} alt="连续试拍画面" onError={() => { onStopTrialStream(); }} /><span className="trial-badge">连续试拍 · JPEG</span></> : trialBusy ? <div className="offline">正在试拍<small>相机正在切换到实际拍摄分辨率</small></div> : <Preview compact online={online} resolution={resolution.preview} />}{previewBlocked && <div className="preview-limit-overlay" role="status"><strong>实时预览已暂停</strong><span>曝光时间超过 100ms，请点击“试拍”查看实际效果。</span></div>}</div><div className="trial-actions"><button type="button" className="primary" disabled={trialBusy || !!trialStreamUrl} onClick={onTrialShot}>{trialBusy ? '试拍中…' : '试拍'}</button><button type="button" className="secondary" aria-label="连续图像流" disabled={trialBusy} onClick={trialStreamUrl ? onStopTrialStream : onStartTrialStream}>{trialStreamUrl ? '停止连续试拍' : '连续试拍'}</button>{trialUrl && <button type="button" className="secondary" onClick={onReturnToLive}>返回实时画面</button>}</div><p className="hint">{trialUrl ? '这是一张实际拍摄分辨率的临时原图，不会保存到项目。' : trialStreamUrl ? '连续试拍通过 JPEG 流更新画面，不会保存照片。' : previewBlocked ? '曝光时间超过实时预览能力，请使用试拍查看实际效果。' : '实时画面用于快速调参；点击“试拍”或“连续试拍”验证实际拍摄效果。'} <span>{saved ? '参数已应用' : '有未保存参数'}</span></p></div><div className="controls"><div className="control-title"><div><p>CAMERA CONTROLS</p><h3>画面参数</h3></div><button type="button" onClick={() => setDraft(configs)}>恢复上次保存</button></div><p className="controls-help">自动控制开启时，相机会持续调整画面；切换为手动后才显示对应的精细参数。</p>{sortControls(draft).filter(config => !isAdvancedControl(config)).map(config => <CameraControl key={config.ID} config={config} draft={draft} setDraft={setDraft} apply={apply} previewing={!trialUrl && !trialStreamUrl} onPreviewLimit={showExposureWarning} />)}<details className="advanced-settings"><summary><span><strong>高级设置</strong><small>压缩质量和底层色彩参数</small></span><b>展开</b></summary><div className="advanced-controls">{sortControls(draft).filter(isAdvancedControl).map(config => <CameraControl key={config.ID} config={config} draft={draft} setDraft={setDraft} apply={apply} previewing={!trialUrl && !trialStreamUrl} onPreviewLimit={showExposureWarning} />)}<p className="advanced-hint">这些参数会影响文件大小或底层色彩处理，通常保持默认即可。</p></div></details><div className="save-hint">调整完成后点击“保存参数”，这些设置会写入拍摄项目。每个参数都可以单独恢复默认值。</div></div></div>}
    {step === 3 && <div className="config"><div><p>STEP 03 / 04</p><h3>设置拍摄节奏</h3><label htmlFor="interval">拍摄间隔<select id="interval" value={intervalMode === 'custom' ? 'custom' : String(interval)} onChange={event => selectInterval(event.target.value)}><option value={60000}>每 1 分钟</option><option value={300000}>每 5 分钟</option><option value={900000}>每 15 分钟</option><option value={3600000}>每 1 小时</option><option value="custom">自定义</option></select></label>{intervalMode === 'custom' && <div className="custom-interval" aria-label="自定义拍摄间隔"><label htmlFor="custom-interval-value">间隔数值<input id="custom-interval-value" type="number" min="1" step="1" inputMode="numeric" value={customIntervalValue} onChange={event => updateCustomInterval(event.target.value)} onBlur={() => { if (!customIntervalValue) updateCustomInterval('1') }} /></label><label htmlFor="custom-interval-unit">单位<select id="custom-interval-unit" value={customIntervalUnit} onChange={event => { const unit = event.target.value as IntervalUnit; setCustomIntervalUnit(unit); updateCustomInterval(customIntervalValue || '1', unit) }}><option value="minutes">分钟</option><option value="seconds">秒</option></select></label><small>自定义间隔至少为 1 秒。</small></div>}<label htmlFor="days">预计拍摄天数<input id="days" type="number" min="1" value={days} onChange={event => setDays(Number(event.target.value))} /></label></div><div className="estimate"><p>ESTIMATE</p><div className="estimate-metrics"><span><strong>{estimate.toLocaleString()}</strong><small>预计照片数量</small></span></div><small className="estimate-note">按拍摄间隔估算照片数量，项目中只保存单张照片。</small></div></div>}
    {step === 4 && <div className="confirm"><b>✓</b><p>STEP 04 / 04</p><h3>准备好开始记录了吗？</h3><p>确认后会创建项目，并将你保存的相机参数应用到这次拍摄。</p><div><span>项目名称<strong>{name || '未命名项目'}</strong></span><span>拍摄间隔<strong>{intervalLabel}</strong></span><span>参数状态<strong>{saved ? '已保存到项目' : '尚有参数未保存'}</strong></span></div></div>}
  </div><footer><button className="secondary" onClick={() => step === 1 ? onClose() : setStep(step - 1)}>{step === 1 ? '取消' : '返回'}</button><span><button className="secondary" onClick={() => setSaved(true)}>保存参数</button>{step < 4 ? <button className="primary" onClick={() => setStep(step + 1)}>继续 →</button> : <button className="primary" disabled={busy} onClick={onCreate}>{busy ? '创建中…' : '创建项目 ✓'}</button>}</span></footer></section>{exposureWarning && <div className="exposure-warning-backdrop" role="presentation"><section className="exposure-warning" role="alertdialog" aria-modal="true" aria-labelledby="exposure-warning-title" tabIndex={-1} onKeyDown={event => { if (event.key === 'Escape') setExposureWarning(false) }}><div className="exposure-warning-icon" aria-hidden="true">!</div><div><p>PREVIEW LIMIT</p><h3 id="exposure-warning-title">实时预览暂不可用</h3><span>曝光时间超过 100ms 后，H.264 预览无法稳定显示。请点击“试拍”查看高曝光参数下的实际照片。</span></div><button type="button" className="primary" autoFocus onClick={() => setExposureWarning(false)}>知道了</button></section></div>}</div>
}

export default App
