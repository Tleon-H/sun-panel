// 检查当前账号图片的批量删除。登录密码只从环境变量读取，不写进输出。
// BASE_URL、E2E_USERNAME、E2E_PASSWORD、E2E_MEMBER_PASSWORD 必填。
// 只会删除这次脚本上传到临时服务里的图片。
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
const stamp = randomBytes(3).toString('hex')
const memberName = `e2e${stamp}`
const pageTwoName = `page-two-${stamp}.png`
const batchNames = Array.from({ length: 12 }, (_, index) => `batch-${String(index + 1).padStart(2, '0')}-${stamp}.png`)
const memberNameFile = `member-keep-${stamp}.png`

if (!baseUrl || !username || !password || !memberPassword) {
  console.error('需要 BASE_URL、E2E_USERNAME、E2E_PASSWORD、E2E_MEMBER_PASSWORD')
  process.exit(1)
}

mkdirSync(shots, { recursive: true })
const api = baseUrl.replace(/\/$/, '')

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

async function apiLogin(account, secret) {
  const response = await fetch(`${api}/api/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: account, password: secret }),
  })
  const body = await response.json()
  if (body.code !== 0 || !body.data?.token)
    throw new Error(`登录失败 ${body.code}`)
  return body.data.token
}

async function uploadImage(token, name, bytes) {
  const form = new FormData()
  form.append('imgfile', new Blob([bytes]), name)
  form.append('category', 'uncategorized')
  const response = await fetch(`${api}/api/file/uploadImg`, {
    method: 'POST',
    headers: { token },
    body: form,
  })
  const body = await response.json()
  if (body.code !== 0)
    throw new Error(`上传失败 ${body.code}`)
}

async function ownFiles(token) {
  const response = await fetch(`${api}/api/file/getList`, {
    method: 'POST',
    headers: { token, 'Content-Type': 'application/json' },
    body: JSON.stringify({ page: 1, limit: 60 }),
  })
  const body = await response.json()
  if (body.code !== 0)
    throw new Error(`读取文件列表失败 ${body.code}`)
  return body.data?.list ?? []
}

async function login(page, account, secret) {
  await page.goto(`${api}/#/login`, { waitUntil: 'domcontentloaded' })
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
  await page.waitForFunction(() => document.body.innerText.includes('系统应用'), { timeout: 8000 })
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

async function selectedCount(page) {
  return page.$eval('[data-testid="selected-count"]', node => node.textContent?.trim() || '')
}

async function clickFilter(page, label) {
  await page.evaluate((name) => {
    const button = [...document.querySelectorAll('.file-filter')].find(node => node.textContent?.trim() === name)
    button?.click()
  }, label)
}

async function clickPage(page, number) {
  await page.evaluate((value) => {
    const button = [...document.querySelectorAll('.file-pages .n-pagination-item')].find(node => node.textContent?.trim() === value)
    button?.click()
  }, String(number))
}

const adminToken = await apiLogin(username, password)
const created = await fetch(`${api}/api/panel/users/create`, {
  method: 'POST',
  headers: { token: adminToken, 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: memberName, password: memberPassword, name: '删除成员', role: 2 }),
})
const createdBody = await created.json()
if (createdBody.code !== 0 && createdBody.code !== 1006)
  throw new Error(`创建普通账号失败 ${createdBody.code}`)

const memberToken = await apiLogin(memberName, memberPassword)
await uploadImage(memberToken, memberNameFile, png(234, 88, 12))
const memberFiles = await ownFiles(memberToken)
const memberFile = memberFiles.find(item => item.fileName === memberNameFile)
if (!memberFile?.id)
  throw new Error('普通账号的图片没有出现在自己的列表里')

await uploadImage(adminToken, pageTwoName, png(22, 163, 74))
await new Promise(resolve => setTimeout(resolve, 1100))
for (const [index, name] of batchNames.entries())
  await uploadImage(adminToken, name, png(37, 40 + index, 235))
note('已在临时库上传 13 张管理员图片和 1 张普通账号图片')

const browser = await puppeteer.launch({
  executablePath: chromePath,
  headless: 'new',
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--lang=zh-CN'],
  defaultViewport: { width: 1280, height: 900 },
})

const deleteBodies = []
try {
  const page = await browser.newPage()
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().includes('/api/file/deletes'))
      deleteBodies.push(request.postData() || '')
  })
  await login(page, username, password)
  await openApp(page, '上传文件管理')
  await page.waitForSelector(`[data-filename="${batchNames[0]}"]`)
  await page.waitForSelector(`[data-testid="select-current-page-hint"]`)
  const hint = await page.$eval('[data-testid="select-current-page-hint"]', node => node.textContent || '')
  if (!hint.includes('这一页') || !hint.includes('其他分页'))
    throw new Error(`选择当前页的说明不清楚：${hint}`)

  await page.click('[data-testid="select-current-page"]')
  await page.waitForFunction(() => document.querySelector('[data-testid="selected-count"]')?.textContent?.includes('12'))
  const selectedOnPage = await page.$$eval('[data-selected="true"]', nodes => nodes.length)
  if (selectedOnPage !== 12)
    throw new Error(`当前页选中了 ${selectedOnPage} 张，不是 12 张`)
  const pageTwoVisible = await page.$(`[data-filename="${pageTwoName}"]`)
  if (pageTwoVisible)
    throw new Error('第二页的图片出现在第一页，无法区分当前页')
  await page.screenshot({ path: `${shots}/batch_delete_select_current_page_desktop.png` })
  note('已保存桌面当前页多选')

  await clickFilter(page, '图标')
  await page.waitForFunction(() => document.querySelector('[data-testid="selected-count"]')?.textContent?.includes('0'))
  await clickFilter(page, '全部')
  await page.waitForSelector(`[data-filename="${batchNames[0]}"]`)
  await clickPage(page, 2)
  await page.waitForSelector(`[data-filename="${pageTwoName}"]`)
  if ((await selectedCount(page)) !== '已选择 0 张')
    throw new Error(`翻页后仍有选择：${await selectedCount(page)}`)
  await clickPage(page, 1)
  await page.waitForSelector(`[data-filename="${batchNames[0]}"]`)
  await page.click('[data-testid="select-current-page"]')
  await page.waitForFunction(() => document.querySelector('[data-testid="selected-count"]')?.textContent?.includes('12'))
  note('搜索分类之外的翻页和分类切换已清空选择')

  await page.click('[data-testid="delete-selected"]')
  await page.waitForFunction(() => {
    const text = document.body.innerText
    return text.includes('删除后无法恢复') && text.includes('引用也会失效') && text.includes('删除已选择的 12 张图片')
  })
  await page.waitForSelector('.n-dialog .n-dialog__action')
  await new Promise(resolve => setTimeout(resolve, 500))
  await page.screenshot({ path: `${shots}/batch_delete_confirm_desktop.png` })
  note('已保存桌面删除确认')

  const beforeCancel = deleteBodies.length
  await page.evaluate(() => {
    const dialog = [...document.querySelectorAll('.n-dialog')].find(node => node.textContent?.includes('删除后无法恢复'))
    const button = [...dialog.querySelectorAll('button')].find(node => node.textContent?.trim() === '取消')
    button?.click()
  })
  await page.waitForFunction(() => !document.body.innerText.includes('删除已选择的 12 张图片'))
  await new Promise(resolve => setTimeout(resolve, 800))
  if (deleteBodies.length !== beforeCancel)
    throw new Error('取消后仍然发出了删除请求')
  if (await page.$(`[data-filename="${batchNames[0]}"]`) === null)
    throw new Error('取消后图片从列表消失了')
  note('取消没有发送删除请求')

  await page.click('[data-testid="delete-selected"]')
  await page.waitForFunction(() => document.body.innerText.includes('删除已选择的 12 张图片'))
  const deleteResponse = page.waitForResponse(response => response.url().includes('/api/file/deletes') && response.request().method() === 'POST')
  await page.evaluate(() => {
    const dialog = [...document.querySelectorAll('.n-dialog')].find(node => node.textContent?.includes('删除后无法恢复'))
    const button = [...dialog.querySelectorAll('button')].find(node => node.textContent?.trim() === '确定')
    button?.click()
  })
  const response = await deleteResponse
  const payload = await response.json()
  if (payload.code !== 0 || (payload.data?.deleted?.length ?? 0) !== 12)
    throw new Error(`删除结果异常 ${payload.code}`)
  await page.waitForFunction(name => {
    return document.body.innerText.includes('已删除 12 张')
      && !document.querySelector('.n-dialog')
      && document.querySelector(`[data-filename="${name}"]`)
  }, {}, pageTwoName)
  await new Promise(resolve => setTimeout(resolve, 400))
  for (const name of batchNames) {
    if (await page.$(`[data-filename="${name}"]`))
      throw new Error(`${name} 删除后仍在列表里`)
  }
  if ((await selectedCount(page)) !== '已选择 0 张')
    throw new Error(`删除后选择没有清空：${await selectedCount(page)}`)
  await page.screenshot({ path: `${shots}/batch_delete_after_desktop.png` })
  note('已保存桌面删除后的列表')

  await page.keyboard.press('Escape')
  await page.waitForFunction(() => !document.querySelector('[data-testid="file-select-bar"]'))
  await openApp(page, '公共图库')
  await page.waitForSelector('[data-testid="public-gallery"]')
  const galleryDeletes = await page.$('[data-testid="public-gallery"] [data-testid="delete-selected"], [data-testid="public-gallery"] [data-testid="file-select"]')
  if (galleryDeletes)
    throw new Error('公共图库出现了删除或批量选择')
  note('公共图库没有删除入口')

  const rejected = await page.evaluate(async (session, id) => {
    const result = await fetch('/api/file/deletes', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ ids: [id], confirm: 'delete-own-files', userId: id }),
    })
    return result.json()
  }, adminToken, memberFile.id)
  const failed = rejected.data?.failed ?? []
  if (rejected.code === 0 || !failed.some(item => item.id === memberFile.id && item.reason === 'not_owner'))
    throw new Error(`跨账号删除没有被拒绝 ${rejected.code}`)
  const stillThere = await ownFiles(memberToken)
  if (!stillThere.some(item => item.id === memberFile.id))
    throw new Error('普通账号的图片被管理员删掉了')
  note('管理员不能删除其他账号的图片')

  await page.setViewport({ width: 390, height: 844, isMobile: true, hasTouch: true })
  await page.reload({ waitUntil: 'networkidle0' })
  await page.waitForFunction(() => document.body.innerText.includes('APP'))
  await openApp(page, '上传文件管理')
  await page.waitForSelector(`[data-filename="${pageTwoName}"]`)
  await page.click(`[data-filename="${pageTwoName}"] [data-testid="file-select"]`)
  await page.waitForFunction(() => document.querySelector('[data-testid="selected-count"]')?.textContent?.includes('1'))
  await page.screenshot({ path: `${shots}/batch_delete_select_mobile.png` })
  note('已保存手机宽度的选择')

  await page.evaluate(() => localStorage.clear())
  await login(page, memberName, memberPassword)
  await page.setViewport({ width: 1280, height: 900, isMobile: false })
  await openApp(page, '上传文件管理')
  await page.waitForSelector(`[data-filename="${memberNameFile}"]`)
  const memberText = await page.$eval('[data-testid="own-file-list"]', node => node.innerText)
  if (memberText.includes(pageTwoName) || memberText.includes(batchNames[0]))
    throw new Error('普通账号看到了管理员的图片')
  await page.screenshot({ path: `${shots}/batch_delete_other_account_remains.png` })
  note('已保存其他账号未被删除的图片')
}
catch (error) {
  const pages = await browser.pages()
  if (pages[0])
    await pages[0].screenshot({ path: '/tmp/leon-batch-delete-failed.png' }).catch(() => {})
  throw error
}
finally {
  await browser.close()
}
