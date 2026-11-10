import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '../..')

function source(path: string): string {
  return readFileSync(join(root, path), 'utf8')
}

function concreteRoutes(routerSource: string): string[] {
  return [...routerSource.matchAll(/path:\s*'([^']+)'/g)]
    .map(match => match[1])
    .filter(path => path !== '/' && !path.includes('pathMatch') && !path.includes(':') && !path.includes('*'))
}

test('hash routes and the Go entry list describe the same pages', () => {
  const routerSource = source('src/router/index.ts')
  const serverSource = source('service/router/web.go')
  assert.match(routerSource, /createWebHashHistory\(/)
  assert.doesNotMatch(routerSource, /createWebHistory\(/)

  const routes = concreteRoutes(routerSource)
  assert.deepEqual(routes, ['/login', '/404', '/500', '/test'])
  const listed = serverSource.match(/hashFrontendRoutes = \[\]string\{([^}]+)\}/)
  assert.ok(listed)
  const serverRoutes = [...listed[1].matchAll(/"([^"]+)"/g)].map(match => match[1])
  assert.deepEqual(serverRoutes, routes)
  assert.doesNotMatch(serverSource, /NoRoute/)
  assert.match(source('service/main.go'), /router\.InitRouters\(":" \+ httpPort\)/)
  assert.doesNotMatch(source('service/main.go'), /127\.0\.0\.1/)
})
