export interface SearchableIcon {
  title?: string | null
  url?: string | null
  description?: string | null
}

export interface IconSearchResult<T, G> {
  active: boolean
  groups: G[]
  matches: T[]
}

export function normalizeSearchKeyword(keyword?: string | null): string {
  return (keyword ?? '').trim().toLowerCase()
}

export function iconMatchesKeyword(item: SearchableIcon, keyword: string): boolean {
  if (!keyword)
    return false
  return [item.title, item.url, item.description].some(field => (field ?? '').toLowerCase().includes(keyword))
}

export function filterIconGroups<T extends SearchableIcon, G extends { items?: T[] | null }>(
  groups: G[],
  keyword: string | undefined | null,
  enabled: boolean,
): IconSearchResult<T, G> {
  const normalized = normalizeSearchKeyword(keyword)
  if (!enabled || normalized === '')
    return { active: false, groups, matches: [] }

  const nextGroups: G[] = []
  const matches: T[] = []
  for (const group of groups) {
    const items = (group.items ?? []).filter(item => iconMatchesKeyword(item, normalized))
    if (items.length === 0)
      continue
    nextGroups.push({ ...group, items } as G)
    matches.push(...items)
  }
  return { active: true, groups: nextGroups, matches }
}

export function moveMatchIndex(current: number, delta: number, count: number): number {
  if (count <= 0)
    return -1
  const start = current < 0 ? (delta < 0 ? count - 1 : 0) : current
  if (current < 0)
    return start
  return (current + delta + count) % count
}

export function decideSearchConfirm(input: {
  keyword?: string | null
  localSearchEnabled: boolean
  matchCount: number
}): 'open-local' | 'open-engine' {
  const keyword = normalizeSearchKeyword(input.keyword)
  if (input.localSearchEnabled && keyword !== '' && input.matchCount > 0)
    return 'open-local'
  return 'open-engine'
}
