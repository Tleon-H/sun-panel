// 用临时服务检查页面流程。登录信息只从环境变量读取，不写进输出。
// BASE_URL、E2E_USERNAME、E2E_PASSWORD 必填。可选 CHROME_PATH、UPLOADS_DIR、BACKUP_OUT。
import { spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createRequire } from 'node:module'
import { randomBytes } from 'node:crypto'

const require = createRequire(import.meta.url)
const puppeteer = require('puppeteer-core')

const baseUrl = process.env.BASE_URL
const username = process.env.E2E_USERNAME
const password = process.env.E2E_PASSWORD
const chromePath = process.env.CHROME_PATH || '/usr/local/bin/google-chrome'
const uploadsDir = process.env.UPLOADS_DIR || ''
const backupOut = process.env.BACKUP_OUT || join(tmpdir(), 'leon-panel-backup-e2e.zip')
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')

if (!baseUrl || !username || !password) {
  console.error('需要 BASE_URL、E2E_USERNAME、E2E_PASSWORD')
  process.exit(1)
}

const work = mkdtempSync(join(tmpdir(), 'leon-flows-'))
const shots = process.env.SHOT_DIR || '/opt/cursor/artifacts'
mkdirSync(shots, { recursive: true })

function image(name) {
  const path = join(work, name)
  writeFileSync(path, png)
  return path
}

const summary = []
function note(message) {
  summary.push(message)
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
  const accountInput = await page.$('input[placeholder="账号"]')
  await accountInput.click({ clickCount: 3 })
  await accountInput.type(account)
  const passwordInput = await page.$('input[placeholder="密码"]')
  await passwordInput.click({ clickCount: 3 })
  await passwordInput.type(secret)
  await page.evaluate(() => {
    const button = [...document.querySelectorAll('button')].find(node => node.textContent?.includes('登录'))
    button?.click()
  })
  await page.waitForFunction(() => location.hash === '#/' && document.body.innerText.includes('APP'), { timeout: 20000 })
}

async function openApps(page) {
  const opened = await page.evaluate(() => document.body.innerText.includes('系统应用'))
  if (!opened) {
    const buttons = await page.$$('.fixed-element button')
    await buttons[buttons.length - 1].click()
    await page.waitForFunction(() => document.body.innerText.includes('系统应用'), { timeout: 10000 })
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
    await page.waitForFunction(label => document.body.innerText.includes(label), { timeout: 10000 }, name)
  }
  await page.evaluate((label) => {
    const target = [...document.querySelectorAll('span, div')].find(node => node.textContent?.trim() === label)
    target?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  }, name)
  await page.waitForFunction(label => document.body.innerText.includes(label), { timeout: 10000 }, name)
}

async function uploadFile(page, filePath) {
  const input = await page.waitForSelector('input[type=file]', { timeout: 10000 })
  const pending = page.waitForResponse(response => response.url().includes('/api/file/uploadImg') && response.request().method() === 'POST')
  await input.uploadFile(filePath)
  const response = await pending
  const payload = await response.json()
  if (payload.code !== 0)
    throw new Error(`上传返回代码 ${payload.code}`)
  return payload.code
}

async function readToken(page) {
  const token = await page.evaluate(() => {
    const raw = localStorage.getItem('AUTH_TOKEN')
    const parsed = raw ? JSON.parse(raw) : null
    return parsed?.data?.token || ''
  })
  if (!token)
    throw new Error('登录后没有拿到会话')
  return token
}

async function searchFiles(page, keyword) {
  const input = await page.waitForSelector('[data-testid="file-name-search"] input')
  await input.click()
  await page.keyboard.down('Control')
  await page.keyboard.press('KeyA')
  await page.keyboard.up('Control')
  await page.keyboard.press('Backspace')
  if (keyword)
    await input.type(keyword)
  await new Promise(resolve => setTimeout(resolve, 500))
}

async function cardText(page, filename) {
  return page.$eval(`[data-filename="${filename}"]`, node => node.innerText)
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
  const token = await readToken(page)
  let removed = 0
  for (let round = 0; round < 5; round += 1) {
    const batch = await page.evaluate(async (session) => {
      const list = await fetch('/api/file/getList', {
        method: 'POST',
        headers: { token: session, 'Content-Type': 'application/json' },
        body: JSON.stringify({ page: 1, limit: 60 }),
      })
      const payload = await list.json()
      const ids = (payload.data?.list || []).map(item => item.id).filter(Boolean)
      if (!ids.length)
        return 0
      const deleted = await fetch('/api/file/deletes', {
        method: 'POST',
        headers: { token: session, 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids }),
      })
      const body = await deleted.json()
      if (body.code !== 0)
        return -1
      return ids.length
    }, token)
    if (batch < 0)
      throw new Error('清理旧示例图片失败')
    removed += batch
    if (batch === 0)
      break
  }
  note(`已清掉临时库里的旧图片 ${removed} 张`)

  await page.waitForSelector('[title="添加"]')
  await page.click('[title="添加"]')
  await page.waitForFunction(() => document.body.innerText.includes('本地上传'))
  await uploadFile(page, image('example-icon.png'))
  note('图标入口上传完成')
  await page.keyboard.press('Escape')
  await new Promise(resolve => setTimeout(resolve, 300))

  await openApp(page, '风格设置')
  await page.waitForFunction(() => document.body.innerText.includes('壁纸'))
  await uploadFile(page, image('example-wallpaper.png'))
  note('壁纸入口上传完成')

  if (!token)
    throw new Error('登录后没有拿到会话')
  for (let index = 1; index <= 12; index += 1) {
    const name = `example-page-${String(index).padStart(2, '0')}.png`
    const bytes = await page.evaluate(async (session, filename, source) => {
      const body = new FormData()
      const binary = Uint8Array.from(atob(source), char => char.charCodeAt(0))
      body.append('imgfile', new Blob([binary], { type: 'image/png' }), filename)
      body.append('category', 'icon')
      const response = await fetch('/api/file/uploadImg', { method: 'POST', headers: { token: session }, body })
      const payload = await response.json()
      return payload.code
    }, token, name, png.toString('base64'))
    if (bytes !== 0)
      throw new Error(`示例图片上传失败 ${bytes}`)
  }
  note('已通过页面同一上传接口补充 12 张示例图')

  await openApp(page, '上传文件管理')
  await searchFiles(page, 'example-icon')
  await page.waitForSelector('[data-filename="example-icon.png"]')
  const iconText = await cardText(page, 'example-icon.png')
  if (!iconText.includes('图标'))
    throw new Error('图标入口的分类不是图标')
  await searchFiles(page, 'example-wallpaper')
  await page.waitForSelector('[data-filename="example-wallpaper.png"]')
  const wallText = await cardText(page, 'example-wallpaper.png')
  if (!wallText.includes('壁纸'))
    throw new Error('壁纸入口的分类不是壁纸')
  note('文件管理按文件名找到图标和壁纸分类')

  await searchFiles(page, 'example-icon')
  await page.waitForSelector('[data-filename="example-icon.png"]')
  await page.click('[data-filename="example-icon.png"] .n-base-selection')
  await page.waitForSelector('.n-base-select-option')
  await page.evaluate(() => {
    const option = [...document.querySelectorAll('.n-base-select-option')].find(node => node.textContent?.includes('壁纸'))
    option?.click()
  })
  await page.waitForFunction(() => document.body.innerText.includes('成功'), { timeout: 10000 })
  note('已在页面把示例图标改为壁纸')

  await page.reload({ waitUntil: 'networkidle0' })
  await page.waitForFunction(() => document.body.innerText.includes('APP'))
  await openApp(page, '上传文件管理')
  await searchFiles(page, 'example-icon')
  await page.waitForSelector('[data-filename="example-icon.png"]')
  const persisted = await cardText(page, 'example-icon.png')
  if (!persisted.includes('壁纸'))
    throw new Error('刷新后分类没有保持为壁纸')
  note('刷新后分类仍然是壁纸')

  await searchFiles(page, 'example-page-07')
  await page.waitForFunction(() => document.querySelectorAll('.file-card').length === 1, { timeout: 10000 })
  const searched = await page.$eval('.file-card', node => node.getAttribute('data-filename'))
  if (searched !== 'example-page-07.png')
    throw new Error(`搜索结果是 ${searched}`)
  note('按文件名搜索只剩对应的一张')

  await searchFiles(page, '')
  await page.waitForSelector('.n-pagination', { timeout: 10000 })
  await page.evaluate(() => {
    const numbers = [...document.querySelectorAll('.n-pagination-item')]
      .map(node => Number(node.textContent?.trim()))
      .filter(number => Number.isFinite(number))
    const last = String(Math.max(...numbers))
    const item = [...document.querySelectorAll('.n-pagination-item')].find(node => node.textContent?.trim() === last)
    item?.click()
  })
  await page.waitForFunction(() => {
    const count = document.querySelectorAll('.file-card').length
    return count > 0 && count < 12
  })
  const secondCount = await page.$$eval('.file-card', nodes => nodes.length)
  note(`分页最后一页显示 ${secondCount} 张`)
  while ((await page.$$('.file-card')).length < 12) {
    const name = await page.$eval('.file-card', node => node.getAttribute('data-filename'))
    const opened = await page.evaluate((filename) => {
      const card = document.querySelector(`[data-filename="${filename}"]`)
      const button = card?.querySelector('button.n-button--error-type')
      button?.click()
      return !!button
    }, name)
    if (!opened)
      throw new Error('没有找到删除按钮')
    await page.waitForSelector('.n-dialog')
    const deleted = page.waitForResponse(response => response.url().includes('/api/file/deletes') && response.request().method() === 'POST')
    const confirmed = await page.evaluate(() => {
      const dialogs = [...document.querySelectorAll('.n-dialog')]
      const dialog = dialogs[dialogs.length - 1]
      const button = [...(dialog?.querySelectorAll('button') || [])].find(item => item.textContent?.trim() === '确定')
      button?.click()
      return !!button
    })
    if (!confirmed)
      throw new Error('删除确认框没有出现')
    const result = await deleted
    const payload = await result.json()
    if (payload.code !== 0)
      throw new Error(`删除返回 ${payload.code}`)
    try {
      await page.waitForFunction(filename => !document.querySelector(`[data-filename="${filename}"]`), { timeout: 10000 }, name)
    }
    catch {
      const left = await page.$$eval('.file-card', nodes => nodes.map(node => node.getAttribute('data-filename')))
      throw new Error(`删除 ${name} 后仍显示 ${left.join(',')}`)
    }
    await page.waitForFunction(() => document.querySelectorAll('.n-dialog').length === 0)
  }
  const afterDelete = await page.$$eval('.file-card', nodes => nodes.length)
  if (afterDelete !== 12)
    throw new Error(`删除末页最后一张后列表变成 ${afterDelete} 张`)
  note('删除末页最后一张后回到仍有图片的一页')

  await page.setViewport({ width: 390, height: 844, isMobile: true })
  await page.reload({ waitUntil: 'networkidle0' })
  await openApp(page, '上传文件管理')
  await page.waitForSelector('.file-card')
  const mobileCards = await page.$$eval('.file-card', nodes => nodes.length)
  if (mobileCards < 1)
    throw new Error('手机宽度下没有显示文件')
  note(`手机宽度下文件管理显示 ${mobileCards} 张`)

  await page.setViewport({ width: 1280, height: 800, isMobile: false })
  await page.reload({ waitUntil: 'networkidle0' })
  await openApp(page, '备份导出')
  await page.waitForSelector('[data-testid="backup-download"]')
  await page.waitForSelector('[data-testid="backup-restore-risk"]')
  await page.waitForFunction(() => document.body.innerText.includes('不要发给别人') && document.body.innerText.includes('覆盖现有数据'))
  await page.screenshot({ path: join(shots, 'leon-backup-desktop.png') })
  note('已保存桌面备份页截图')

  const download = page.waitForResponse(response => response.url().includes('/api/backup/export'))
  await page.click('[data-testid="backup-download"]')
  const exported = await download
  const archive = await exported.buffer()
  if (!exported.ok() || !String(exported.headers()['content-type'] || '').includes('application/zip'))
    throw new Error(`备份响应不是压缩包，状态 ${exported.status()}`)
  writeFileSync(backupOut, archive, { mode: 0o600 })
  const verify = spawnSync('python3', ['-c', `
import hashlib, json, os, sqlite3, sys, tempfile, zipfile
path, uploads = sys.argv[1], sys.argv[2]
archive = zipfile.ZipFile(path)
names = archive.namelist()
if any(name.startswith("/") or ".." in name.split("/") for name in names):
    raise SystemExit("archive path escaped")
manifest = json.loads(archive.read("manifest.json"))
assert manifest["version"] == "1.3.0+leon.8", manifest.get("version")
assert manifest["database"]["method"] == "sqlite-vacuum-into"
assert manifest["database"]["integrity"] == "ok"
assert manifest["database"]["included"] is True
assert manifest["restore"] == "overwrite"
assert manifest["compatibleWithOfficial181"] is False
assert any("密码哈希" in item for item in manifest["warnings"])
for item in manifest["files"]:
    body = archive.read(item["path"])
    assert hashlib.sha256(body).hexdigest() == item["sha256"]
    assert len(body) == item["size"]
snap = tempfile.NamedTemporaryFile(delete=False, suffix=".sqlite")
snap.write(archive.read("database/database.sqlite"))
snap.close()
db = sqlite3.connect(snap.name)
integrity = db.execute("pragma integrity_check").fetchone()[0]
rows = dict(db.execute("select file_name, category from file where id in (select max(id) from file where file_name like 'example-%' group by file_name)"))
db.close()
os.remove(snap.name)
if uploads and os.path.isdir(uploads):
    for root, dirs, files in os.walk(uploads):
        for name in files:
            if name.endswith(".zip"):
                raise SystemExit("zip stored in uploads")
print(json.dumps({"integrity": integrity, "files": len(manifest["files"]), "rows": rows}, ensure_ascii=False))
`, backupOut, uploadsDir], { encoding: 'utf8' })
  if (verify.status !== 0) {
    console.error(verify.stderr)
    throw new Error('备份包校验失败')
  }
  const report = JSON.parse(verify.stdout)
  if (report.integrity !== 'ok')
    throw new Error('快照完整性不是 ok')
  if (report.rows['example-icon.png'] !== 'wallpaper' || report.rows['example-wallpaper.png'] !== 'wallpaper')
    throw new Error('快照里的示例分类不正确')
  if (!report.rows['example-page-07.png'])
    throw new Error('快照里没有搜索用的示例图')
  note(`备份包可打开，完整性 ${report.integrity}，清单 ${report.files} 个文件，示例分类已写入快照`)
  rmSync(backupOut, { force: true })

  await page.setViewport({ width: 390, height: 844, isMobile: true })
  await page.reload({ waitUntil: 'networkidle0' })
  await openApp(page, '备份导出')
  await page.waitForSelector('[data-testid="backup-download"]')
  await page.screenshot({ path: join(shots, 'leon-backup-mobile.png') })
  note('已保存手机备份页截图')

  const visitor = await page.evaluate(async (origin) => {
    const root = origin.replace(/\/$/, '')
    const exported = await fetch(`${root}/api/backup/export`, { method: 'POST' })
    const restored = await fetch(`${root}/api/backup/restore`, { method: 'POST' })
    return { exportCode: (await exported.json()).code, restoreCode: (await restored.json()).code }
  }, baseUrl.replace(/\/$/, ''))
  if (visitor.exportCode !== 1000 || visitor.restoreCode !== 1000)
    throw new Error(`访客返回 ${visitor.exportCode}/${visitor.restoreCode}`)
  note('未登录导出和恢复被拒绝')

  const regularPassword = randomBytes(12).toString('hex')
  const regular = await page.evaluate(async (session, secret) => {
    const created = await fetch('/api/panel/users/create', {
      method: 'POST',
      headers: { token: session, 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: 'e2eregular', password: secret, name: 'e2e', role: 2 }),
    })
    const payload = await created.json()
    return payload.code
  }, token, regularPassword)
  if (regular !== 0)
    throw new Error(`创建普通账号失败 ${regular}`)
  await page.evaluate(() => localStorage.clear())
  await login(page, 'e2eregular', regularPassword)
  await openApps(page)
  const seesBackup = await page.evaluate(() => document.body.innerText.includes('备份导出'))
  if (seesBackup)
    throw new Error('普通账号看到了备份入口')
  const denied = await page.evaluate(async () => {
    const raw = localStorage.getItem('AUTH_TOKEN')
    const session = JSON.parse(raw).data.token
    const exported = await fetch('/api/backup/export', { method: 'POST', headers: { token: session } })
    const restored = await fetch('/api/backup/restore', { method: 'POST', headers: { token: session } })
    return { exportCode: (await exported.json()).code, restoreCode: (await restored.json()).code }
  })
  if (denied.exportCode !== 1005 || denied.restoreCode !== 1005)
    throw new Error(`普通账号导出/恢复返回 ${denied.exportCode}/${denied.restoreCode}`)
  note('普通账号没有入口，导出和恢复被拒绝')
  console.log('全部页面检查通过')
}
finally {
  await browser.close()
  rmSync(work, { recursive: true, force: true })
  rmSync(backupOut, { force: true })
}
