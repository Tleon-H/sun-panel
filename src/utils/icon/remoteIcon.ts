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

// A new address or icon name starts a fresh attempt. An empty image address stays on fallback.
export function phaseAfterTargetChange(itemType?: number | null, src?: string | null, text?: string | null): RemoteIconPhase {
  if (itemType === 2)
    return (src ?? '').trim() ? 'loading' : 'fallback'
  if (itemType === 3)
    return (text ?? '').trim() ? 'loading' : 'fallback'
  return 'loading'
}

export function acceptIconEvent(phase: RemoteIconPhase, event: 'success' | 'error', requestId: number, currentId: number): RemoteIconPhase {
  if (requestId !== currentId)
    return phase
  return reduceRemoteIconPhase(phase, event)
}

export interface RemoteIconTarget {
  itemType?: number | null
  src?: string | null
  text?: string | null
}

export interface RemoteIconController {
  readonly phase: RemoteIconPhase
  readonly requestId: number
  sync: () => { requestId: number; phase: RemoteIconPhase; shouldLoadIcon: boolean }
  onImageEvent: (event: 'success' | 'error', requestId: number) => RemoteIconPhase
  onIconLoad: (event: 'success' | 'error', requestId: number) => RemoteIconPhase
}

export function createRemoteIconController(getTarget: () => RemoteIconTarget): RemoteIconController {
  let requestId = 0
  let phase: RemoteIconPhase = 'loading'
  return {
    get phase() {
      return phase
    },
    get requestId() {
      return requestId
    },
    sync() {
      requestId += 1
      const target = getTarget()
      phase = phaseAfterTargetChange(target.itemType, target.src, target.text)
      return {
        requestId,
        phase,
        shouldLoadIcon: target.itemType === 3 && phase === 'loading',
      }
    },
    onImageEvent(event, id) {
      phase = acceptIconEvent(phase, event, id, requestId)
      return phase
    },
    onIconLoad(event, id) {
      phase = acceptIconEvent(phase, event, id, requestId)
      return phase
    },
  }
}
