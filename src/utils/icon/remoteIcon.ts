export type RemoteIconPhase = 'loading' | 'ready' | 'fallback'

export function isRemoteIconType(itemType?: number | null): boolean {
  return itemType === 2 || itemType === 3
}

// Failure is terminal for the current icon value so a broken URL cannot refresh itself.
export function reduceRemoteIconPhase(phase: RemoteIconPhase, event: 'success' | 'error'): RemoteIconPhase {
  if (phase === 'fallback')
    return 'fallback'
  if (event === 'error')
    return 'fallback'
  return 'ready'
}

export function fallbackLabel(itemType: number | undefined, customText?: string | null, title?: string | null): string {
  const text = (customText ?? '').trim()
  const name = (title ?? '').trim()
  const source = itemType === 3 ? (name || text) : (text || name)
  if (!source)
    return '?'
  return Array.from(source)[0]
}

export function rejectAfter(ms: number): Promise<never> {
  return new Promise((_resolve, reject) => {
    setTimeout(() => reject(new Error('timeout')), ms)
  })
}
