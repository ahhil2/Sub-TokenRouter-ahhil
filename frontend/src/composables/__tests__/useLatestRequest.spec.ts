import { effectScope } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useLatestRequest } from '../useLatestRequest'

/**
 * useLatestRequest.spec.ts verifies request replacement, cancellation, scope disposal, and current-error propagation.
 * Controlled requests and browser substitutes isolate the observable state transitions
 * so fixtures can reproduce delayed completion and cleanup without external services.
 */

/**
 * deferred exposes one controllable promise for explicit completion-order assertions.
 */
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

describe('useLatestRequest', () => {
  // A previous loader settles after a replacement request has already started. Both stale
  // success and stale failure must leave the current result and loading state untouched.
  it.each(['resolve', 'reject'] as const)(
    '丢弃旧请求的 %s，加载状态由当前请求决定',
    async (finish) => {
      const scope = effectScope()
      const request = scope.run(useLatestRequest)!
      const old = deferred<string>()
      const current = deferred<string>()
      let oldSignal!: AbortSignal
      const first = request.run((signal) => {
        oldSignal = signal
        return old.promise
      })
      const second = request.run(() => current.promise)
      expect(oldSignal.aborted).toBe(true)
      old[finish]('stale')
      expect(await first).toBeUndefined()
      expect(request.loading.value).toBe(true)
      current.resolve('current')
      expect(await second).toBe('current')
      expect(request.loading.value).toBe(false)
      scope.stop()
    },
  )

  // Cancellation or scope disposal interrupts a loader that ignores its signal. Its late
  // result must be discarded, while only explicit cancellation permits another run.
  it.each(['cancel', 'dispose'])(
    '%s 会中止请求，忽略不响应 AbortSignal 的加载器',
    async (action) => {
      const scope = effectScope()
      const request = scope.run(useLatestRequest)!
      const response = deferred<string>()
      let signal!: AbortSignal
      const pending = request.run((value) => {
        signal = value
        return response.promise
      })
      if (action === 'cancel') request.cancel()
      else scope.stop()
      response.resolve('stale')
      expect(await pending).toBeUndefined()
      expect(signal.aborted).toBe(true)
      expect(request.loading.value).toBe(false)
      const next = vi.fn().mockResolvedValue('next')
      expect(await request.run(next)).toBe(action === 'cancel' ? 'next' : undefined)
      scope.stop()
    },
  )

  // The active loader fails while related work could still use its signal. Failure must
  // abort that signal, clear loading, and propagate the current error.
  it('当前请求失败时取消同组任务并传递错误', async () => {
    const scope = effectScope()
    const request = scope.run(useLatestRequest)!
    let signal!: AbortSignal
    await expect(
      request.run(async (value) => {
        signal = value
        throw new Error('failed')
      }),
    ).rejects.toThrow('failed')
    expect(signal.aborted).toBe(true)
    expect(request.loading.value).toBe(false)
    scope.stop()
  })
})
