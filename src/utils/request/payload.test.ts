import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import axios from 'axios'
import { prepareHttpRequest } from './payload.ts'

const requestSource = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'index.ts'), 'utf8')

function formWithTwoImages() {
  const form = new FormData()
  form.append('files[]', new File(['blue-bytes'], 'blue.png', { type: 'image/png' }))
  form.append('files[]', new File(['green-bytes'], 'green.png', { type: 'image/png' }))
  form.append('category', 'uncategorized')
  return form
}

function listen() {
  const seen: { type: string, body: string, token: string }[] = []
  const server = createServer((req, res) => {
    const chunks: Buffer[] = []
    req.on('data', chunk => chunks.push(chunk))
    req.on('end', () => {
      seen.push({
        type: String(req.headers['content-type'] || ''),
        body: Buffer.concat(chunks).toString('latin1'),
        token: String(req.headers.token || ''),
      })
      res.setHeader('content-type', 'application/json')
      res.end(JSON.stringify({ code: 0, data: {} }))
    })
  })
  return new Promise<{ url: string, seen: typeof seen, close: () => Promise<void> }>((resolve) => {
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      if (!address || typeof address === 'string')
        throw new Error('test server did not bind')
      resolve({
        url: `http://127.0.0.1:${address.port}/file/uploadFiles`,
        seen,
        close: () => new Promise((done, reject) => server.close(error => error ? reject(error) : done())),
      })
    })
  })
}

test('the request wrapper keeps FormData and does not flatten it', () => {
  assert.match(requestSource, /prepareHttpRequest\(/)
  assert.doesNotMatch(requestSource, /Object\.assign\(/)
})

test('a FormData request stays multipart and keeps both files and the category', async () => {
  const form = formWithTwoImages()
  const flattened = Object.assign({}, form)
  assert.equal(Object.keys(flattened).length, 0)
  assert.equal(JSON.stringify(form), '{}')

  const prepared = prepareHttpRequest({
    data: () => form,
    headers: { 'Content-Type': 'application/json', token: 'forged' },
    token: 'session',
    lang: 'zh-CN',
  })
  assert.equal(prepared.data, form)
  assert.equal(prepared.headers['Content-Type'], undefined)
  assert.equal(prepared.headers['content-type'], undefined)
  assert.equal(prepared.headers.token, 'session')
  assert.equal(prepared.headers.lang, 'zh-CN')
  const names = [...(prepared.data as FormData).entries()].map(([name, value]) => {
    return [name, typeof value === 'string' ? value : value.name]
  })
  assert.deepEqual(names, [
    ['files[]', 'blue.png'],
    ['files[]', 'green.png'],
    ['category', 'uncategorized'],
  ])

  const server = await listen()
  try {
    await axios.post(server.url, prepared.data, { headers: prepared.headers })
  }
  finally {
    await server.close()
  }
  assert.equal(server.seen.length, 1)
  const sent = server.seen[0]
  assert.match(sent.type, /^multipart\/form-data;\s*boundary=/i)
  assert.doesNotMatch(sent.type, /application\/json/i)
  assert.doesNotMatch(sent.type, /application\/x-www-form-urlencoded/i)
  assert.match(sent.body, /name="files\[\]"; filename="blue.png"/)
  assert.match(sent.body, /name="files\[\]"; filename="green.png"/)
  assert.match(sent.body, /blue-bytes/)
  assert.match(sent.body, /green-bytes/)
  assert.match(sent.body, /name="category"/)
  assert.match(sent.body, /uncategorized/)
  assert.equal(sent.token, 'session')
})

test('an ordinary object is still posted as JSON', async () => {
  const prepared = prepareHttpRequest({
    data: { page: 1, category: 'icon' },
    token: 'session',
    lang: 'zh-CN',
  })
  assert.equal(prepared.data instanceof FormData, false)
  const server = await listen()
  try {
    await axios.post(server.url, prepared.data, { headers: prepared.headers })
  }
  finally {
    await server.close()
  }
  const sent = server.seen[0]
  assert.match(sent.type, /application\/json/i)
  assert.deepEqual(JSON.parse(sent.body), { page: 1, category: 'icon' })
})
