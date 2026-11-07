import assert from 'node:assert/strict'
import test from 'node:test'
import { attachMonitorPoll, createPoller, firstNumber } from './pollLifecycle.ts'

function deferred() {
  let resolve: () => void = () => {}
  const promise = new Promise<void>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

test('stopping a poller clears the timer and ignores a sample that returns later', async () => {
  const pending = deferred()
  const timers: Array<() => void> = []
  let cleared = 0
  let applied = 0
  const poller = createPoller({
    intervalMs: 5000,
    run: async (isCurrent) => {
      await pending.promise
      if (isCurrent())
        applied += 1
    },
    setIntervalFn: (handler) => {
      timers.push(handler)
      return timers.length
    },
    clearIntervalFn: () => {
      cleared += 1
    },
  })

  poller.start()
  assert.equal(timers.length, 1)
  poller.stop()
  assert.equal(cleared, 1)
  assert.equal(poller.stopped, true)
  pending.resolve()
  await pending.promise
  await Promise.resolve()
  assert.equal(applied, 0)
})

test('a new sample does not start while the previous sample is still running', async () => {
  const pending = deferred()
  const timers: Array<() => void> = []
  let calls = 0
  const poller = createPoller({
    intervalMs: 5000,
    run: async () => {
      calls += 1
      await pending.promise
    },
    setIntervalFn: (handler) => {
      timers.push(handler)
      return timers.length
    },
    clearIntervalFn: () => {},
  })

  poller.start()
  timers[0]()
  assert.equal(calls, 1)
  pending.resolve()
  await pending.promise
  await Promise.resolve()
  poller.stop()
})

test('the monitor card hook starts on mount and drops a sample that finishes after unmount', async () => {
  const pending = deferred()
  let mounted = () => {}
  let unmounted = () => {}
  let applied = 0
  attachMonitorPoll({
    intervalMs: 5000,
    run: async (isCurrent) => {
      await pending.promise
      if (isCurrent())
        applied += 1
    },
    onMounted: (hook) => {
      mounted = hook
    },
    onUnmounted: (hook) => {
      unmounted = hook
    },
    create: options => createPoller({
      ...options,
      setIntervalFn: () => 1,
      clearIntervalFn: () => {},
    }),
  })

  mounted()
  unmounted()
  pending.resolve()
  await pending.promise
  await Promise.resolve()
  assert.equal(applied, 0)
})

test('cpu usage falls back to zero when the sample list is missing', () => {
  assert.equal(firstNumber(undefined), 0)
  assert.equal(firstNumber(null), 0)
  assert.equal(firstNumber([]), 0)
  assert.equal(firstNumber([12.5]), 12.5)
})
