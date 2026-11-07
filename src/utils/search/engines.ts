export interface SearchEngineRecord {
  id: string
  title: string
  url: string
  builtin: boolean
}

export interface SearchConfig {
  engines: SearchEngineRecord[]
  currentId: string
  newWindowOpen: boolean
}

export type SearchConfigError = 'title' | 'protocol' | 'duplicate' | 'builtin' | 'missing' | 'limit'

export type SearchConfigResult =
  | { ok: true; config: SearchConfig }
  | { ok: false; error: SearchConfigError }

export const builtinSearchEngines: SearchEngineRecord[] = [
  { id: 'google', title: 'Google', url: 'https://www.google.com/search?q=%s', builtin: true },
  { id: 'baidu', title: 'Baidu', url: 'https://www.baidu.com/s?wd=%s', builtin: true },
  { id: 'bing', title: 'Bing', url: 'https://www.bing.com/search?q=%s', builtin: true },
]

const MAX_ENGINES = 20
const MAX_TITLE = 40
const MAX_URL = 1000

export function defaultSearchConfig(): SearchConfig {
  return {
    engines: builtinSearchEngines.map(engine => ({ ...engine })),
    currentId: builtinSearchEngines[0].id,
    newWindowOpen: false,
  }
}

export function validateSearchTitle(raw: string): string | null {
  const title = raw.trim()
  if (!title || title.length > MAX_TITLE || title.includes('<') || title.includes('>'))
    return null
  return title
}

// %s is the only placeholder. It is replaced after the keyword is URL-encoded.
// A template without %s is still allowed: the encoded keyword is appended.
export function validateSearchTemplate(raw: string): string | null {
  const template = raw.trim()
  if (!template || template.length > MAX_URL || /[\s<>"'`]/.test(template) || template.includes('\\'))
    return null
  const probe = template.split('%s').join('keyword')
  let url: URL
  try {
    url = new URL(probe)
  }
  catch {
    return null
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:')
    return null
  if (/^[a-z][a-z0-9+.-]*:/i.test(template) === false)
    return null
  return template
}

export function buildSearchUrl(template: string, keyword: string): string | null {
  const checked = validateSearchTemplate(template)
  if (!checked)
    return null
  const encoded = encodeURIComponent(keyword)
  if (checked.includes('%s'))
    return checked.split('%s').join(encoded)
  if (!keyword)
    return checked
  return checked + encoded
}

function builtinByUrl(url: string): SearchEngineRecord | undefined {
  return builtinSearchEngines.find(engine => engine.url === url)
}

function builtinById(id: string): SearchEngineRecord | undefined {
  return builtinSearchEngines.find(engine => engine.id === id)
}

function copyConfig(config: SearchConfig): SearchConfig {
  return {
    currentId: config.currentId,
    newWindowOpen: config.newWindowOpen,
    engines: config.engines.map(engine => ({ ...engine })),
  }
}

function withCurrent(config: SearchConfig, preferred?: string): SearchConfig {
  const next = copyConfig(config)
  if (preferred && next.engines.some(engine => engine.id === preferred))
    next.currentId = preferred
  else if (!next.engines.some(engine => engine.id === next.currentId))
    next.currentId = next.engines[0]?.id ?? builtinSearchEngines[0].id
  if (next.engines.length === 0)
    return defaultSearchConfig()
  return next
}

function readRecord(value: unknown, fallbackId: string): SearchEngineRecord | null {
  if (!value || typeof value !== 'object')
    return null
  const row = value as Record<string, unknown>
  const rawUrl = typeof row.url === 'string' ? row.url : ''
  const matched = builtinByUrl(rawUrl) ?? (typeof row.id === 'string' ? builtinById(row.id) : undefined)
  if (matched && (row.builtin === true || matched.url === rawUrl || row.id === matched.id))
    return { ...matched }
  const title = validateSearchTitle(typeof row.title === 'string' ? row.title : '')
  const url = validateSearchTemplate(rawUrl)
  if (!title || !url || builtinByUrl(url))
    return null
  const id = typeof row.id === 'string' && /^custom-[a-z0-9-]+$/i.test(row.id) ? row.id : fallbackId
  return { id, title, url, builtin: false }
}

export function normalizeSearchConfig(raw: unknown): SearchConfig {
  if (!raw || typeof raw !== 'object')
    return defaultSearchConfig()
  const source = raw as Record<string, unknown>
  const newWindowOpen = source.newWindowOpen === true
  const hasEngines = Object.prototype.hasOwnProperty.call(source, 'engines') && Array.isArray(source.engines)
  const incoming = hasEngines
    ? source.engines as unknown[]
    : [
        ...builtinSearchEngines,
        ...(Array.isArray(source.searchEngineList) ? source.searchEngineList : []),
      ]
  const engines: SearchEngineRecord[] = []
  const seen = new Set<string>()
  incoming.forEach((item, index) => {
    const record = readRecord(item, `custom-restored-${index + 1}`)
    if (!record || seen.has(record.url) || engines.length >= MAX_ENGINES)
      return
    seen.add(record.url)
    engines.push(record)
  })
  const currentRecord = source.currentSearchEngine && typeof source.currentSearchEngine === 'object'
    ? source.currentSearchEngine as Record<string, unknown>
    : undefined
  const currentUrl = typeof currentRecord?.url === 'string' ? currentRecord.url : undefined
  const currentFromUrl = typeof currentUrl === 'string'
    ? engines.find(engine => engine.url === currentUrl)?.id
    : undefined
  const preferred = typeof source.currentId === 'string' ? source.currentId : currentFromUrl
  const base = engines.length > 0
    ? { engines, currentId: preferred ?? engines[0].id, newWindowOpen }
    : { ...defaultSearchConfig(), newWindowOpen }
  const preferredOnBase = preferred ?? (typeof currentUrl === 'string'
    ? base.engines.find(engine => engine.url === currentUrl)?.id
    : undefined)
  const normalized = withCurrent(base, preferredOnBase)
  if (typeof currentUrl === 'string' && !normalized.engines.some(engine => engine.url === currentUrl)) {
    const extra = readRecord(currentRecord, 'custom-current')
    if (extra?.builtin)
      return { ...normalized, currentId: extra.id }
    if (extra)
      return { ...normalized, engines: [...normalized.engines, extra], currentId: extra.id }
  }
  return normalized
}

export function toPersistedSearchConfig(config: SearchConfig): Record<string, unknown> {
  const normalized = withCurrent(config)
  const current = normalized.engines.find(engine => engine.id === normalized.currentId) ?? normalized.engines[0]
  return {
    engines: normalized.engines.map(engine => ({
      id: engine.id,
      title: engine.title,
      url: engine.url,
      builtin: engine.builtin,
    })),
    currentId: current.id,
    currentSearchEngine: {
      title: current.title,
      url: current.url,
      iconSrc: '',
    },
    searchEngineList: normalized.engines
      .filter(engine => !engine.builtin)
      .map(engine => ({ title: engine.title, url: engine.url, iconSrc: '' })),
    newWindowOpen: normalized.newWindowOpen,
  }
}

export function currentSearchEngine(config: SearchConfig): SearchEngineRecord {
  return config.engines.find(engine => engine.id === config.currentId) ?? config.engines[0] ?? builtinSearchEngines[0]
}

export function selectEngine(config: SearchConfig, id: string): SearchConfig {
  return withCurrent(config, id)
}

function duplicateUrl(config: SearchConfig, url: string, exceptId?: string): boolean {
  return config.engines.some(engine => engine.url === url && engine.id !== exceptId)
}

export function addCustomEngine(config: SearchConfig, title: string, url: string, id = `custom-${Math.random().toString(36).slice(2, 10)}`): SearchConfigResult {
  const safeTitle = validateSearchTitle(title)
  const safeUrl = validateSearchTemplate(url)
  if (!safeTitle)
    return { ok: false, error: 'title' }
  if (!safeUrl)
    return { ok: false, error: 'protocol' }
  if (config.engines.length >= MAX_ENGINES)
    return { ok: false, error: 'limit' }
  if (duplicateUrl(config, safeUrl) || builtinByUrl(safeUrl))
    return { ok: false, error: 'duplicate' }
  const next = copyConfig(config)
  next.engines.push({ id, title: safeTitle, url: safeUrl, builtin: false })
  return { ok: true, config: next }
}

export function updateCustomEngine(config: SearchConfig, id: string, title: string, url: string): SearchConfigResult {
  const index = config.engines.findIndex(engine => engine.id === id)
  if (index < 0)
    return { ok: false, error: 'missing' }
  if (config.engines[index].builtin)
    return { ok: false, error: 'builtin' }
  const safeTitle = validateSearchTitle(title)
  const safeUrl = validateSearchTemplate(url)
  if (!safeTitle)
    return { ok: false, error: 'title' }
  if (!safeUrl)
    return { ok: false, error: 'protocol' }
  if (duplicateUrl(config, safeUrl, id) || builtinByUrl(safeUrl))
    return { ok: false, error: 'duplicate' }
  const next = copyConfig(config)
  next.engines[index] = { id, title: safeTitle, url: safeUrl, builtin: false }
  return { ok: true, config: next }
}

export function removeCustomEngine(config: SearchConfig, id: string): SearchConfigResult {
  const target = config.engines.find(engine => engine.id === id)
  if (!target)
    return { ok: false, error: 'missing' }
  if (target.builtin)
    return { ok: false, error: 'builtin' }
  const next = copyConfig(config)
  next.engines = next.engines.filter(engine => engine.id !== id)
  return { ok: true, config: withCurrent(next) }
}

export function moveEngine(config: SearchConfig, index: number, delta: number): SearchConfig {
  const nextIndex = index + delta
  if (index < 0 || index >= config.engines.length || nextIndex < 0 || nextIndex >= config.engines.length)
    return config
  const next = copyConfig(config)
  const [item] = next.engines.splice(index, 1)
  next.engines.splice(nextIndex, 0, item)
  return next
}
