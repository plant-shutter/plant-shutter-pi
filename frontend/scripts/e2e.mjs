import { chromium } from 'playwright'
import { spawn } from 'node:child_process'

const baseURL = 'https://127.0.0.1:5173'
process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0'
const projectName = `e2e-${Date.now()}`
let devServer

async function ensureDevServer() {
  try {
    await fetch(baseURL)
    return
  } catch {}

  devServer = spawn('npm', ['run', 'dev', '--', '--host', '127.0.0.1'], {
    cwd: new URL('..', import.meta.url),
    stdio: 'inherit',
    detached: false,
  })
  for (let attempt = 0; attempt < 50; attempt += 1) {
    try {
      await fetch(baseURL)
      return
    } catch {
      await new Promise(resolve => setTimeout(resolve, 200))
    }
  }
  throw new Error('Vite dev server did not start on https://127.0.0.1:5173')
}

await ensureDevServer()
const browser = await chromium.launch({ headless: true, args: ['--disable-gpu', '--no-first-run'] })
const context = await browser.newContext({ ignoreHTTPSErrors: true, locale: 'zh-CN', viewport: { width: 390, height: 844 }, deviceScaleFactor: 1 })
const page = await context.newPage()
page.on('dialog', dialog => dialog.accept())
const failures = []
page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
page.on('console', message => { if (message.type() === 'error') failures.push(`console: ${message.text()}`) })
page.on('request', request => { if (request.url().includes('/api/')) console.log('REQ', request.method(), request.url()) })
page.on('response', response => { if (response.url().includes('/api/')) console.log('RES', response.status(), response.url()) })

async function expectVisible(selector, label) {
  const locator = page.locator(selector)
  await locator.first().waitFor({ state: 'visible', timeout: 15000 }).catch(() => { throw new Error(`${label} is not visible`) })
}

try {
  await page.goto(baseURL, { waitUntil: 'networkidle' })
  const existing = await page.request.get(`${baseURL}/api/project`).then(response => response.json())
  const running = await page.request.get(`${baseURL}/api/project/running`).then(response => response.json())
  if (running.data?.name) await page.request.put(`${baseURL}/api/project`, { data: { name: running.data.name, running: false } })
  for (const project of existing.data ?? []) {
    if (project.running) await page.request.put(`${baseURL}/api/project`, { data: { name: project.name, running: false } })
    if (String(project.name).startsWith('e2e-')) await page.request.delete(`${baseURL}/api/project/${encodeURIComponent(project.name)}`)
  }
  await page.reload({ waitUntil: 'networkidle' })
  await expectVisible('h1', 'dashboard heading')
  await expectVisible('text=实时画面', 'current view')
  await page.screenshot({ path: '/tmp/plant-shutter-home-mobile.png', fullPage: true })

  await page.getByRole('button', { name: /创建拍摄项目/ }).first().click()
  await expectVisible('.create-page', 'project creation page')
  await page.getByLabel('项目名称').fill(projectName)
  await page.getByRole('button', { name: /继续/ }).click()
  await expectVisible('text=画面参数', 'camera tuning step')
  await page.getByRole('button', { name: '试拍' }).click()
  await expectVisible('text=试拍原图', 'trial shot result')
  await page.getByRole('button', { name: '返回实时画面' }).click()
  const slider = page.locator('input[type=range]').first()
  await slider.focus()
  await slider.press('ArrowRight')
  await slider.dispatchEvent('mouseup')
  await page.getByRole('button', { name: '保存参数' }).click()
  await expectVisible('text=参数已应用', 'saved camera state')
  await page.getByRole('button', { name: /继续/ }).click()
  await expectVisible('text=设置拍摄节奏', 'capture schedule step')
  await page.getByRole('button', { name: /继续/ }).click()
  await expectVisible(`text=${projectName}`, 'project confirmation')
  await page.getByRole('button', { name: /创建项目/ }).last().click()
  await page.locator('.create-page').waitFor({ state: 'hidden', timeout: 15000 })
  await expectVisible('text=你的项目', 'project list')
  await expectVisible(`text=${projectName}`, 'created project')

  await page.getByRole('button', { name: new RegExp(projectName) }).click()
  await expectVisible('text=打开照片库', 'project detail actions')
  await page.getByRole('button', { name: /继续拍摄/ }).click()
  await expectVisible('text=拍摄中', 'running project state')
  await page.getByRole('button', { name: /暂停拍摄/ }).click()
  await expectVisible('text=已暂停', 'paused project state')
  await page.getByRole('button', { name: /结束项目/ }).click()
  await expectVisible('text=已完成', 'completed project state')
  await page.getByRole('button', { name: /打开照片库/ }).click()
  await expectVisible('text=PHOTO LIBRARY', 'photo gallery')
  await page.screenshot({ path: '/tmp/plant-shutter-gallery-mobile.png', fullPage: true })

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.reload({ waitUntil: 'networkidle' })
  await expectVisible('h1', 'desktop heading')
  await page.screenshot({ path: '/tmp/plant-shutter-home-desktop.png', fullPage: true })
  if (failures.length) throw new Error(failures.join('\n'))
  console.log(JSON.stringify({ ok: true, projectName, screenshots: ['/tmp/plant-shutter-home-mobile.png', '/tmp/plant-shutter-gallery-mobile.png', '/tmp/plant-shutter-home-desktop.png'] }))
} finally {
  await page.request.delete(`http://192.168.2.91:8081/api/project/${encodeURIComponent(projectName)}`).catch(() => {})
  await browser.close()
  if (devServer) devServer.kill('SIGTERM')
}
