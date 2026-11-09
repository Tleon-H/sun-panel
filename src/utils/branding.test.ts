import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '../..')

function source(path: string): string {
  return readFileSync(join(root, path), 'utf8')
}

test('the visible product name is Leon Panel and the version stays 1.3.0+leon.8', () => {
  assert.equal(source('service/assets/version').trim(), '10|1.3.0+leon.8')

  const html = source('index.html')
  assert.match(html, /<title>Leon Panel<\/title>/)
  assert.match(html, /name="application-name" content="Leon Panel"/)
  assert.match(html, /name="apple-mobile-web-app-title" content="Leon Panel"/)

  const manifest = source('vite.config.ts')
  assert.match(manifest, /name: 'Leon Panel'/)
  assert.match(manifest, /short_name: 'Leon Panel'/)
  assert.doesNotMatch(manifest, /Sun-Panel/)

  for (const path of ['src/locales/zh-CN.json', 'src/locales/en-US.json']) {
    const copy = source(path)
    assert.match(copy, /"appName": "Leon Panel"/)
    assert.match(copy, /Sun-Panel v1\.3\.0/)
    assert.doesNotMatch(copy, /QQ交流群|QQ Group|打赏|Donate/)
  }

  const panel = source('src/store/modules/panel/helper.ts')
  assert.match(panel, /logoText: 'Leon Panel'/)
  assert.match(panel, /Leon Panel · 基于开源/)
  assert.match(panel, /github\.com\/hslr-s\/sun-panel/)

  const login = source('src/views/login/index.vue')
  assert.match(login, /\$t\('common\.appName'\)/)
  assert.match(login, /setTitle\(t\('common\.appName'\)\)/)
  assert.match(login, /Sun-Panel<\/a> v1\.3\.0/)

  const about = source('src/components/apps/About/index.vue')
  assert.match(about, /\$t\('common\.appName'\)/)
  assert.match(about, /alt="Leon Panel"/)
  assert.match(about, /github\.com\/hslr-s\/sun-panel/)
  assert.match(about, /github\.com\/Tleon-H\/sun-panel/)
  assert.doesNotMatch(about, /donate|qq\.com|hub\.docker\.com|gitee\.com|bilibili|youtube/i)

  const readme = source('README.md')
  assert.match(readme, /^# Leon Panel/m)
  assert.match(readme, /1\.3\.0\+leon\.8/)
  assert.match(readme, /内部数字版本：`10`/)
  assert.match(readme, /整站覆盖恢复/)
  assert.match(readme, /OpenAPI/)
  assert.match(readme, /Docker 套接字控制/)
  assert.match(readme, /批量删除/)
  assert.match(readme, /\[LICENSE\]\(\.\/LICENSE\)/)
  assert.match(readme, /https:\/\/github\.com\/hslr-s\/sun-panel/)
  assert.doesNotMatch(readme, /star-history|hub\.docker\.com|doc\.sun-panel\.top|enianteam\.com|qq_group|donate/)
})
