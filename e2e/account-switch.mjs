// 检查本机多账号切换。登录密码只从环境变量读取，不写进输出。
// BASE_URL、E2E_USERNAME、E2E_PASSWORD 必填。E2E_MEMBER_PASSWORD 是临时普通账号的密码。
import { mkdirSync } from 'node:fs'
import { createRequire } from 'node:module'
import { randomBytes } from 'node:crypto'

const require = createRequire(import.meta.url)
const puppeteer = require('puppeteer-core')

const baseUrl = process.env.BASE_URL
const username = process.env.E2E_USERNAME
const password = process.env.E2E_PASSWORD
const memberPassword = process.env.E2E_MEMBER_PASSWORD
const chromePath = process.env.CHROME_PATH || '/usr/local/bin/google-chrome'
const shots = process.env.SHOT_DIR || '/opt/cursor/artifacts'
const memberName = 'e2emember'

if (!baseUrl || !username || !password || !memberPassword) {
  console.error('需要 BASE_URL、E2E_USERNAME、E2E_PASSWORD、E2E_MEMBER_PASSWORD')
  process.exit(1)
}

mkdirSync(shots, { recursive: true })

function note(message) {
  console.log(message)
}

async function login(page, account, secret) {
  await page.goto(`${baseUrl.replace(/\/$/, '')}/#/login`, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => {
    localStorage.setItem('appSetting', JSON.stringify({
      data: { siderCollapsed: false, theme: 'light', language: 'zh-CN' },
      expire: null,
    }))
  })
  await page.reload({ waitUntil: 'networkidle0' })
  await page.waitForSelector('input[placeholder="账号"]')
  await page.click('input[placeholder="账号"]', { clickCount: 3 })
  await page.type('input[placeholder="账号"]', account)
  await page.click('input[placeholder="密码"]', { clickCount: 3 })
  await page.type('input[placeholder="密码"]', secret)
  await page.evaluate(() => {
    const button = [...document.querySelectorAll('button')].find(node => node.textContent?.includes('登录') && !node.textContent?.includes('另一个'))
    button?.click()
  })
  await page.waitForFunction(() => location.hash === '#/' && document.body.innerText.includes('APP'), { timeout: 20000 })
}

async function openApps(page) {
  const opened = await page.evaluate(() => document.body.innerText.includes('系统应用'))
  if (!opened) {
    const buttons = await page.$$('.fixed-element button')
    await buttons[buttons.length - 1].click()
    await page.waitForFunction(() => document.body.innerText.includes('系统应用'))
  }
}

async function openApp(page, name) {
  await openApps(page)
  const visible = await page.evaluate(label => document.body.innerText.includes(label), name)
  if (!visible) {
    await page.evaluate(() => {
      const header = [...document.querySelectorAll('div')].find(node => node.textContent?.trim() === '系统应用')
      header?.click()
    })
    await page.waitForFunction(label => document.body.innerText.includes(label), {}, name)
  }
  await page.evaluate((label) => {
    const target = [...document.querySelectorAll('span, div')].find(node => node.textContent?.trim() === label)
    target?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  }, name)
  await page.waitForFunction(label => document.body.innerText.includes(label), {}, name)
}

async function readToken(page) {
  return page.evaluate(() => {
    const raw = localStorage.getItem('AUTH_TOKEN')
    const parsed = raw ? JSON.parse(raw) : null
    return parsed?.data?.token || ''
  })
}

const browser = await puppeteer.launch({
  executablePath: chromePath,
  headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--lang=zh-CN'],
  defaultViewport: { width: 1280, height: 800 },
})

try {
  const page = await browser.newPage()
  page.setDefaultTimeout(20000)
  await login(page, username, password)
  note('管理员已登录')
  const adminToken = await readToken(page)
  const created = await page.evaluate(async (session, secret, account) => {
    const response = await fetch('/api/panel/users/create', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: account, password: secret, name: '示例成员', role: 2 }),
    })
    const body = await response.json()
    return body.code
  }, adminToken, memberPassword, memberName)
  if (created !== 0 && created !== 1006)
    throw new Error(`创建普通账号失败 ${created}`)
  const png = [137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13]
  const uploaded = await page.evaluate(async (session, bytes) => {
    const file = new File([new Uint8Array(bytes)], 'admin-only.png', { type: 'image/png' })
    const form = new FormData()
    form.append('imgfile', file)
    form.append('category', 'icon')
    const response = await fetch('/api/file/uploadImg', { method: 'POST', headers: { token: session }, body: form })
    const body = await response.json()
    return body.code
  }, adminToken, png)
  if (uploaded !== 0)
    throw new Error(`管理员上传失败 ${uploaded}`)

  await openApp(page, '切换账号')
  await page.waitForSelector('[data-testid="account-switch"]')
  const warning = await page.$eval('[data-testid="account-switch"]', node => node.innerText)
  if (!warning.includes('不保存密码') || !warning.includes('访客'))
    throw new Error('切换账号说明不完整')
  async function fillControl(testId, value) {
    const handle = await page.evaluateHandle((id) => {
      const root = document.querySelector(`[data-testid="${id}"]`)
      if (!root)
        return null
      return root.matches('input,textarea') ? root : root.querySelector('input,textarea')
    }, testId)
    const element = handle.asElement()
    if (!element)
      throw new Error(`没有找到输入框 ${testId}`)
    await element.click({ clickCount: 3 })
    await element.type(value)
  }
  await fillControl('account-switch-username', memberName)
  await fillControl('account-switch-password', memberPassword)
  const typed = await page.evaluate(() => {
    const read = (id) => {
      const root = document.querySelector(`[data-testid="${id}"]`)
      const input = root?.matches('input,textarea') ? root : root?.querySelector('input,textarea')
      return input ? input.value.length : -1
    }
    return { username: read('account-switch-username'), password: read('account-switch-password') }
  })
  if (typed.username !== memberName.length || typed.password < 1)
    throw new Error(`输入框长度 username=${typed.username} password=${typed.password}`)
  await page.click('[data-testid="account-switch-add"]')
  await page.waitForFunction(previous => {
    const raw = localStorage.getItem('AUTH_TOKEN')
    const parsed = raw ? JSON.parse(raw) : null
    const token = parsed?.data?.token || ''
    return token && token !== previous
  }, { timeout: 20000 }, adminToken)
  await page.waitForFunction(() => location.hash === '#/' || location.hash === '#')
  note('已从管理员切换到普通账号')

  const memberToken = await readToken(page)
  if (!memberToken || memberToken === adminToken)
    throw new Error('切换后仍是原来的会话')
  const memberFiles = await page.evaluate(async (session) => {
    const response = await fetch('/api/file/getList', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ page: 1, limit: 24 }),
    })
    const body = await response.json()
    return { code: body.code, names: (body.data?.list || []).map(item => item.fileName) }
  }, memberToken)
  if (memberFiles.code !== 0 || memberFiles.names.includes('admin-only.png'))
    throw new Error('普通账号看到了管理员的图片')
  const memberBackup = await page.evaluate(async (session) => {
    const response = await fetch('/api/backup/export', { method: 'POST', headers: { token: session } })
    const body = await response.json()
    return body.code
  }, memberToken)
  if (memberBackup !== 1005)
    throw new Error(`普通账号备份返回 ${memberBackup}`)
  await openApps(page)
  const memberSeesBackup = await page.evaluate(() => document.body.innerText.includes('备份导出'))
  if (memberSeesBackup)
    throw new Error('普通账号看到了备份入口')
  note('普通账号看不到管理员的图片和备份入口')

  await openApp(page, '切换账号')
  await page.waitForSelector(`[data-username="${username}"]`)
  const back = page.waitForNavigation({ waitUntil: 'networkidle0' })
  await page.click(`[data-username="${username}"]`)
  await back
  await page.waitForFunction(() => document.body.innerText.includes('APP'))
  const adminAgain = await readToken(page)
  if (adminAgain !== adminToken)
    throw new Error('没有回到原来的管理员会话')
  await openApps(page)
  const adminSeesBackup = await page.evaluate(() => document.body.innerText.includes('备份导出'))
  if (!adminSeesBackup)
    throw new Error('回到管理员后没有备份入口')
  const adminFiles = await page.evaluate(async (session) => {
    const response = await fetch('/api/file/getList', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ page: 1, limit: 24 }),
    })
    const body = await response.json()
    return (body.data?.list || []).map(item => item.fileName)
  }, adminAgain)
  if (!adminFiles.includes('admin-only.png'))
    throw new Error('管理员自己的图片不在了')
  note('切回管理员后仍只看到自己的图片')

  await openApp(page, '切换账号')
  await page.waitForFunction(() => document.body.innerText.includes('不保存密码'))
  const stored = await page.evaluate(() => {
    const raw = localStorage.getItem('LEON_ACCOUNT_SESSIONS')
    const parsed = raw ? JSON.parse(raw) : null
    const list = Array.isArray(parsed?.data) ? parsed.data : []
    return list.map(item => Object.keys(item).sort())
  })
  if (stored.length < 2 || stored.some(keys => keys.includes('password') || !keys.includes('token')))
    throw new Error('浏览器保存的账号列表不正确')
  await page.screenshot({ path: `${shots}/leon-account-switch-desktop.png` })
  await page.setViewport({ width: 390, height: 844, isMobile: true, hasTouch: true })
  await page.reload({ waitUntil: 'networkidle0' })
  await page.waitForFunction(() => document.body.innerText.includes('APP'))
  await openApp(page, '切换账号')
  await page.waitForFunction(() => document.body.innerText.includes('不保存密码'))
  await page.screenshot({ path: `${shots}/leon-account-switch-mobile.png` })
  note('已保存切换账号截图')

  await page.evaluate(() => localStorage.clear())
  const visitor = await page.evaluate(async (origin) => {
    const response = await fetch(`${origin}/api/file/getList`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ page: 1, limit: 24 }),
    })
    const body = await response.json()
    return body.code
  }, baseUrl.replace(/\/$/, ''))
  if (visitor !== 1000)
    throw new Error(`访客列出文件返回 ${visitor}`)
  note('未登录不能列出文件')
  note('账号切换检查通过')
}
finally {
  await browser.close()
}
