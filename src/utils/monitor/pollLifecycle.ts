export interface PollRun {
  (isCurrent: () => boolean): Promise<void> | void
}

export interface Poller {
  start: () => void
  stop: () => void
  readonly stopped: boolean
  readonly inFlight: boolean
}

export function createPoller(options: {
  intervalMs: number
  minIntervalMs?: number
  run: PollRun
  setIntervalFn?: (handler: () => void, timeout: number) => unknown
  clearIntervalFn?: (handle: unknown) => void
}): Poller {
  const minIntervalMs = options.minIntervalMs ?? 2000
  const intervalMs = Math.max(minIntervalMs, options.intervalMs || 0)
  const schedule = options.setIntervalFn ?? ((handler, timeout) => setInterval(handler, timeout))
  const cancel = options.clearIntervalFn ?? (handle => clearInterval(handle as ReturnType<typeof setInterval>))

  let timer: unknown = null
  let generation = 0
  let inFlight = false
  let stopped = true

  async function tick() {
    if (stopped || inFlight)
      return
    inFlight = true
    const ticket = generation
    const isCurrent = () => !stopped && generation === ticket
    try {
      await options.run(isCurrent)
    }
    catch {
      // Keep the card in place when a sample fails.
    }
    finally {
      inFlight = false
    }
  }

  return {
    get stopped() {
      return stopped
    },
    get inFlight() {
      return inFlight
    },
    start() {
      if (!stopped)
        return
      stopped = false
      void tick()
      timer = schedule(() => {
        void tick()
      }, intervalMs)
    },
    stop() {
      stopped = true
      generation += 1
      if (timer !== null) {
        cancel(timer)
        timer = null
      }
    },
  }
}

// The Vue cards pass their real onMounted and onUnmounted hooks here.
export function attachMonitorPoll(options: {
  intervalMs: number
  run: PollRun
  onMounted: (hook: () => void) => void
  onUnmounted: (hook: () => void) => void
  create?: typeof createPoller
}): Poller {
  const poller = (options.create ?? createPoller)({
    intervalMs: options.intervalMs,
    run: options.run,
  })
  options.onMounted(() => {
    poller.start()
  })
  options.onUnmounted(() => {
    poller.stop()
  })
  return poller
}

export function firstNumber(values?: Array<number | null> | null): number {
  const value = values?.[0]
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}
