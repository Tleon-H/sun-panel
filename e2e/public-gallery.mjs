// 检查公共图库。登录密码只从环境变量读取，不写进输出。
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
const memberName = 'e2egallery'
const stamp = randomBytes(3).toString('hex')
const publicName = `gallery-public-${stamp}.png`
const privateName = `gallery-private-${stamp}.png`
const memberFile = `member-private-${stamp}.png`
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')

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

async function upload(page, token, filename, category) {
  const code = await page.evaluate(async (session, name, categoryName, bytes) => {
    const file = new File([Uint8Array.from(bytes)], name, { type: 'image/png' })
    const form = new FormData()
    form.append('imgfile', file)
    form.append('category', categoryName)
    const response = await fetch('/api/file/uploadImg', { method: 'POST', headers: { token: session }, body: form })
    const body = await response.json()
    return body.code
  }, token, filename, category, [...png])
  if (code !== 0)
    throw new Error(`上传 ${filename} 失败 ${code}`)
}

const browser = await puppeteer.launch({
  executablePath: chromePath,
  headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--lang=zh-CN'],
  defaultViewport: { width: 1280, height: 800 },
})

try {
  const page = await browser.newPage()
  await login(page, username, password)
  const adminToken = await readToken(page)
  const created = await page.evaluate(async (session, secret, account) => {
    const response = await fetch('/api/panel/users/create', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: account, password: secret, name: '图库成员', role: 2 }),
    })
    const body = await response.json()
    return body.code
  }, adminToken, memberPassword, memberName)
  if (created !== 0 && created !== 1006)
    throw new Error(`创建普通账号失败 ${created}`)
  await upload(page, adminToken, publicName, 'icon')
  await upload(page, adminToken, privateName, 'icon')

  await openApp(page, '上传文件管理')
  await page.waitForSelector(`[data-filename="${publicName}"] [data-testid="file-public-toggle"]`)
  const before = await page.$eval(`[data-filename="${publicName}"]`, node => node.getAttribute('data-public'))
  if (before !== 'false')
    throw new Error('新上传的图片默认不是私人的')
  await page.click(`[data-filename="${publicName}"] [data-testid="file-public-toggle"]`)
  await page.waitForFunction(name => document.querySelector(`[data-filename="${name}"]`)?.getAttribute('data-public') === 'true', {}, publicName)
  const privateFlag = await page.$eval(`[data-filename="${privateName}"]`, node => node.getAttribute('data-public'))
  if (privateFlag !== 'false')
    throw new Error('没有标记的图片变成了公共')
  note('管理员只公开了自己的一张图片')

  await openApp(page, '公共图库')
  await page.waitForSelector(`[data-testid="public-gallery"] [data-filename="${publicName}"]`)
  const adminGallery = await page.$eval('[data-testid="public-gallery"]', node => node.innerText)
  if (!adminGallery.includes('只显示管理员公开') || adminGallery.includes(privateName))
    throw new Error('管理员的公共图库混入了私人图片')
  await page.screenshot({ path: `${shots}/leon-public-gallery-desktop.png` })
  note('已保存桌面端公共图库')

  await page.evaluate(() => localStorage.clear())
  await login(page, memberName, memberPassword)
  const memberToken = await readToken(page)
  await upload(page, memberToken, memberFile, 'wallpaper')
  await openApp(page, '上传文件管理')
  await page.waitForSelector(`[data-filename="${memberFile}"]`)
  const memberCanPublish = await page.$('[data-testid="file-public-toggle"]')
  if (memberCanPublish)
    throw new Error('普通账号看到了公共标记按钮')
  const memberFiles = await page.evaluate(async (session) => {
    const response = await fetch('/api/file/getList', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ page: 1, limit: 24, userId: 1 }),
    })
    const body = await response.json()
    return (body.data?.list || []).map(item => item.fileName)
  }, memberToken)
  if (memberFiles.includes(publicName) || memberFiles.includes(privateName) || !memberFiles.includes(memberFile))
    throw new Error('普通账号的文件列表越权了')

  await openApp(page, '公共图库')
  await page.waitForSelector(`[data-testid="public-gallery"] [data-filename="${publicName}"]`)
  const memberGallery = await page.$eval('[data-testid="public-gallery"]', node => node.innerText)
  if (memberGallery.includes(privateName) || memberGallery.includes(memberFile))
    throw new Error('公共图库显示了未公开的图片')
  const ids = await page.evaluate(async (session, memberFileName) => {
    const own = await fetch('/api/file/getList', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    })
    const ownBody = await own.json()
    const ownId = (ownBody.data?.list || []).find(item => item.fileName === memberFileName)?.id
    const denied = await fetch('/api/file/setPublic', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: ownId, public: true, userId: 1 }),
    })
    const deniedBody = await denied.json()
    const gallery = await fetch('/api/file/publicList', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ userId: 1, user_id: 1 }),
    })
    const galleryBody = await gallery.json()
    const names = (galleryBody.data?.list || []).map(item => item.fileName)
    const leakedOwner = (galleryBody.data?.list || []).some(item => Object.hasOwn(item, 'userId'))
    return { denied: deniedBody.code, names, leakedOwner }
  }, memberToken, memberFile)
  if (ids.denied !== 1005 || ids.leakedOwner || ids.names.includes(privateName) || ids.names.includes(memberFile) || !ids.names.includes(publicName))
    throw new Error(`公共图库权限不正确 ${ids.denied}`)
  const chosen = await page.evaluate(async (session, name) => {
    const image = document.querySelector(`[data-filename="${name}"] img`)
    const src = image?.getAttribute('src') || ''
    const buttons = [...document.querySelectorAll(`[data-filename="${name}"] button`)]
    buttons[buttons.length - 1]?.click()
    await new Promise(resolve => setTimeout(resolve, 800))
    const response = await fetch('/api/panel/userConfig/get', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: '{}',
    })
    const body = await response.json()
    const saved = body.data?.panel?.backgroundImageSrc || ''
    const matched = Boolean(src) && (saved === src || saved.endsWith(src))
    return { matched, hasImage: Boolean(src), hasSaved: Boolean(saved) }
  }, memberToken, publicName)
  if (!chosen.matched)
    throw new Error('选用公开图片后没有记到自己的壁纸')
  note('普通账号能看到公开图片，不能公开或读取私人图片，选用只改自己的壁纸')

  await page.setViewport({ width: 390, height: 844, isMobile: true, hasTouch: true })
  await page.reload({ waitUntil: 'networkidle0' })
  await page.waitForFunction(() => document.body.innerText.includes('APP'))
  await openApp(page, '公共图库')
  await page.waitForSelector('[data-testid="public-gallery"]')
  await page.screenshot({ path: `${shots}/leon-public-gallery-mobile.png` })
  note('已保存手机宽度公共图库')

  await page.evaluate(() => localStorage.clear())
  const visitor = await page.evaluate(async (origin) => {
    const response = await fetch(`${origin}/api/file/publicList`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ userId: 1 }),
    })
    const body = await response.json()
    return body.code
  }, baseUrl.replace(/\/$/, ''))
  if (visitor !== 1000)
    throw new Error(`访客列出公共图库返回 ${visitor}`)
  note('未登录不能列出公共图库')
  note('公共图库检查通过')
}
finally {
  await browser.close()
}
