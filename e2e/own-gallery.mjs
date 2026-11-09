// 检查批量上传和自己的图库。登录密码只从环境变量读取，不写进输出。
// BASE_URL、E2E_USERNAME、E2E_PASSWORD、E2E_MEMBER_PASSWORD 必填。
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
const memberName = 'e2eown'
const stamp = randomBytes(3).toString('hex')
const blueName = `own-blue-${stamp}.png`
const greenName = `own-green-${stamp}.png`
const orangeName = `member-orange-${stamp}.png`

if (!baseUrl || !username || !password || !memberPassword) {
  console.error('需要 BASE_URL、E2E_USERNAME、E2E_PASSWORD、E2E_MEMBER_PASSWORD')
  process.exit(1)
}

mkdirSync(shots, { recursive: true })

function note(message) {
  console.log(message)
}

function png(red, green, blue) {
  const width = 64
  const height = 64
  const raw = Buffer.alloc((width * 3 + 1) * height)
  for (let y = 0; y < height; y++) {
    const row = y * (width * 3 + 1)
    raw[row] = 0
    for (let x = 0; x < width; x++) {
      raw[row + 1 + x * 3] = red
      raw[row + 2 + x * 3] = green
      raw[row + 3 + x * 3] = blue
    }
  }
  const { deflateSync, crc32 } = require('node:zlib')
  const chunk = (type, data) => {
    const body = Buffer.concat([Buffer.from(type), data])
    const length = Buffer.alloc(4)
    length.writeUInt32BE(data.length)
    const sum = Buffer.alloc(4)
    sum.writeUInt32BE(crc32(body) >>> 0)
    return Buffer.concat([length, body, sum])
  }
  const header = Buffer.alloc(13)
  header.writeUInt32BE(width, 0)
  header.writeUInt32BE(height, 4)
  header[8] = 8
  header[9] = 2
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk('IHDR', header),
    chunk('IDAT', deflateSync(raw)),
    chunk('IEND', Buffer.alloc(0)),
  ])
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
  if (opened)
    return
  await page.waitForFunction(() => !document.querySelector('.n-modal-mask'))
  const buttons = await page.$$('.fixed-element button')
  await buttons[buttons.length - 1].click()
  try {
    await page.waitForFunction(() => document.body.innerText.includes('系统应用'), { timeout: 8000 })
  }
  catch (error) {
    await page.screenshot({ path: `${shots}/leon-open-apps-failed.png` })
    throw error
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

async function closeApps(page) {
  await page.keyboard.press('Escape')
  await page.waitForFunction(() => !document.body.innerText.includes('系统应用'))
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
  const bluePath = `/tmp/${blueName}`
  const greenPath = `/tmp/${greenName}`
  const { writeFileSync } = await import('node:fs')
  writeFileSync(bluePath, png(37, 99, 235))
  writeFileSync(greenPath, png(22, 163, 74))

  await login(page, username, password)
  await openApp(page, '上传文件管理')
  await page.waitForSelector('[data-testid="batch-upload-input"]')
  const input = await page.$('[data-testid="batch-upload-input"]')
  await input.uploadFile(bluePath, greenPath)
  await page.waitForFunction((blue, green) => {
    return document.body.innerText.includes('已上传 2 张')
      && document.querySelector(`[data-filename="${blue}"]`)
      && document.querySelector(`[data-filename="${green}"]`)
  }, {}, blueName, greenName)
  const categories = await page.$$eval('.file-card .n-base-selection-input__content', nodes => nodes.map(node => node.textContent?.trim()))
  if (!categories.includes('未分类'))
    throw new Error(`批量上传后的分类不是未分类：${categories.join(',')}`)
  await page.screenshot({ path: `${shots}/leon-batch-upload-desktop.png` })
  note('已保存桌面批量上传')

  await page.click(`[data-filename="${blueName}"] [data-testid="file-public-toggle"]`)
  await page.waitForFunction(name => document.querySelector(`[data-filename="${name}"]`)?.getAttribute('data-public') === 'true', {}, blueName)
  const greenPrivate = await page.$eval(`[data-filename="${greenName}"]`, node => node.getAttribute('data-public'))
  if (greenPrivate !== 'false')
    throw new Error('没有标记的图片变成了公共')
  note('管理员只公开了自己的一张图片')

  await closeApps(page)
  await page.waitForSelector('span[title="添加"]')
  await page.evaluate(() => {
    document.querySelector('span[title="添加"]')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
  await page.waitForSelector('[data-testid="open-own-icons"]')
  await page.click('[data-testid="open-own-icons"]')
  await page.waitForSelector(`[data-testid="own-gallery"] [data-filename="${greenName}"]`)
  const ownText = await page.$eval('[data-testid="own-gallery"]', node => node.innerText)
  if (!ownText.includes('只显示当前账号') || !ownText.includes(blueName) || !ownText.includes(greenName))
    throw new Error('自己的图库没有列出当前账号的图片')
  await page.screenshot({ path: `${shots}/leon-own-gallery-icon-desktop.png` })
  note('已保存桌面自己的图库')
  await page.evaluate((name) => {
    document.querySelector(`[data-testid="own-gallery"] [data-filename="${name}"] [data-testid="own-gallery-pick"]`)?.click()
  }, greenName)
  await page.waitForFunction(() => !document.querySelector('[data-testid="own-gallery"]'))
  const iconSrc = await page.$eval('input[placeholder="输入图标地址或上传"]', node => node.value)
  if (!iconSrc.includes('/uploads/'))
    throw new Error('选用自己的图片后图标地址没有变化')
  const title = await page.$('.n-modal input[maxlength="20"]')
  await title.click({ clickCount: 3 })
  await title.type('示例项目')
  const url = await page.$('input[placeholder="http(s)://"]')
  await url.click({ clickCount: 3 })
  await url.type('https://example.com')
  const savedItem = page.waitForResponse(response => response.url().includes('/api/panel/itemIcon/edit'))
  await page.evaluate(() => {
    const button = [...document.querySelectorAll('button')].find(node => node.textContent?.trim() === '保存')
    button?.click()
  })
  const itemResponse = await savedItem
  const itemBody = await itemResponse.json()
  const savedSrc = itemBody.data?.icon?.src || ''
  if (itemBody.code !== 0 || !savedSrc.includes('/uploads/'))
    throw new Error(`保存图标失败 ${itemBody.code}`)
  await page.waitForFunction(() => !document.body.innerText.includes('添加项目'))
  note('已把自己的图片选为项目图标')

  await openApp(page, '风格设置')
  await page.waitForSelector('[data-testid="open-own-wallpapers"]')
  await page.click('[data-testid="open-own-wallpapers"]')
  await page.waitForSelector(`[data-testid="own-gallery"] [data-filename="${blueName}"]`)
  await page.screenshot({ path: `${shots}/leon-own-wallpaper-desktop.png` })
  await page.evaluate((name) => {
    document.querySelector(`[data-testid="own-gallery"] [data-filename="${name}"] [data-testid="own-gallery-pick"]`)?.click()
  }, blueName)
  await page.waitForFunction(() => document.body.innerText.includes('配置已保存'), { timeout: 10000 })
  const token = await readToken(page)
  const wallpaper = await page.evaluate(async (session) => {
    const response = await fetch('/api/panel/userConfig/get', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: '{}',
    })
    const body = await response.json()
    return body.data?.panel?.backgroundImageSrc || ''
  }, token)
  if (!wallpaper.includes('/uploads/'))
    throw new Error('选用自己的图片后壁纸没有保存')
  note('已把自己的图片选为壁纸')

  await page.setViewport({ width: 390, height: 844, isMobile: true, hasTouch: true })
  await page.reload({ waitUntil: 'networkidle0' })
  await page.waitForFunction(() => document.body.innerText.includes('APP'))
  await openApp(page, '上传文件管理')
  await page.waitForSelector(`[data-filename="${greenName}"]`)
  await page.screenshot({ path: `${shots}/leon-batch-upload-mobile.png` })
  note('已保存手机宽度文件管理')
  await closeApps(page)
  await openApp(page, '风格设置')
  await page.waitForSelector('[data-testid="open-own-wallpapers"]')
  await page.click('[data-testid="open-own-wallpapers"]')
  await page.waitForSelector('[data-testid="own-gallery"]')
  await page.screenshot({ path: `${shots}/leon-own-gallery-mobile.png` })
  note('已保存手机宽度自己的图库')

  await page.setViewport({ width: 1280, height: 800, isMobile: false })
  await page.keyboard.press('Escape')
  await page.keyboard.press('Escape')
  const created = await page.evaluate(async (session, secret, account) => {
    const response = await fetch('/api/panel/users/create', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: account, password: secret, name: '图库成员', role: 2 }),
    })
    const body = await response.json()
    return body.code
  }, token, memberPassword, memberName)
  if (created !== 0 && created !== 1006)
    throw new Error(`创建普通账号失败 ${created}`)

  await page.evaluate(() => localStorage.clear())
  await login(page, memberName, memberPassword)
  const memberToken = await readToken(page)
  const uploaded = await page.evaluate(async (session, name, bytes) => {
    const file = new File([Uint8Array.from(bytes)], name, { type: 'image/png' })
    const form = new FormData()
    form.append('imgfile', file)
    form.append('category', 'icon')
    const response = await fetch('/api/file/uploadImg', { method: 'POST', headers: { token: session }, body: form })
    const body = await response.json()
    return body.code
  }, memberToken, orangeName, [...png(234, 88, 12)])
  if (uploaded !== 0)
    throw new Error(`普通账号上传失败 ${uploaded}`)

  await openApp(page, '上传文件管理')
  await page.waitForSelector(`[data-filename="${orangeName}"]`)
  const memberFiles = await page.evaluate(async (session) => {
    const response = await fetch('/api/file/getList', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ page: 1, limit: 24, userId: 1 }),
    })
    const body = await response.json()
    return (body.data?.list || []).map(item => item.fileName)
  }, memberToken)
  if (memberFiles.includes(blueName) || memberFiles.includes(greenName) || !memberFiles.includes(orangeName))
    throw new Error('普通账号的文件列表混入了别人的图片')
  const managerText = await page.$eval('[data-testid="own-file-list"]', node => node.innerText)
  if (managerText.includes(blueName) || managerText.includes(greenName))
    throw new Error('普通账号在上传文件管理里看到了别人的图片')
  await page.screenshot({ path: `${shots}/leon-own-files-member-desktop.png` })
  note('普通账号的文件管理只有自己的图片')

  await openApp(page, '风格设置')
  await page.click('[data-testid="open-own-wallpapers"]')
  await page.waitForSelector(`[data-testid="own-gallery"] [data-filename="${orangeName}"]`)
  const memberOwn = await page.$eval('[data-testid="own-gallery"]', node => node.innerText)
  if (memberOwn.includes(blueName) || memberOwn.includes(greenName))
    throw new Error('自己的图库混入了别人的公开或私人图片')
  await page.keyboard.press('Escape')
  await openApp(page, '公共图库')
  await page.waitForSelector(`[data-testid="public-gallery"] [data-filename="${blueName}"]`)
  const publicText = await page.$eval('[data-testid="public-gallery"]', node => node.innerText)
  if (publicText.includes(greenName) || publicText.includes(orangeName))
    throw new Error('公共图库显示了未公开的图片')
  note('普通账号看不到别人的私人图片，公共图库仍只显示已公开的图片')
  note('自己的图库检查通过')
}
finally {
  await browser.close()
}
