import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createCachedRequest, invalidateReadCaches } from '../readCache'

/**
 * readCache.spec.ts verifies shared request caching, ownership changes, expiry, and editable result isolation.
 * Controlled requests and browser substitutes isolate the observable state transitions
 * so fixtures can reproduce delayed completion and cleanup without external services.
 */

const language = vi.hoisted(() => ({ current: 'zh' }))
vi.mock('@/i18n', () => ({ getLocale: () => language.current }))

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

beforeEach(() => {
  localStorage.clear()
  language.current = 'zh'
  invalidateReadCaches()
})
afterEach(() => vi.useRealTimers())

describe('配置读取缓存', () => {
  // Two callers overlap one configuration read and edit the returned options independently.
  // The assertions protect request deduplication and independent cached and caller-owned
  // copies.
  it('合并并发读取，表单各自持有可编辑的副本', async () => {
    const response = deferred<{ options: number[] }>()
    const load = vi.fn(() => response.promise)
    const read = createCachedRequest(load)
    const first = read()
    const second = read()
    response.resolve({ options: [1] })
    const [a, b] = await Promise.all([first, second])
    a.options.push(2)
    expect(b.options).toEqual([1])
    expect((await read()).options).toEqual([1])
    expect(load).toHaveBeenCalledOnce()
  })

  // A configuration read fails before later succeeding and expiring under a controlled
  // clock. The reader must retry failures and refresh values after their lifetime.
  it('失败后可以重试，过期后重新读取', async () => {
    vi.useFakeTimers()
    const load = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(1)
    const read = createCachedRequest(load, 1000)
    await expect(read()).rejects.toThrow('offline')
    expect(await read()).toBe(1)
    await vi.advanceTimersByTimeAsync(1001)
    await read()
    expect(load).toHaveBeenCalledTimes(3)
  })

  // Force refresh, mutation, credential changes, and locale changes each replace request
  // ownership. A delayed earlier result must not overwrite the newer cached response.
  it.each(['force', 'mutation', 'account', 'language'])(
    '%s 后迟到的结果不会覆盖当前缓存',
    async (mode) => {
      const old = deferred<number>()
      const load = vi
        .fn()
        .mockImplementationOnce(() => old.promise)
        .mockResolvedValue(2)
      const read = createCachedRequest<number>(load)
      const pending = read()
      await Promise.resolve()
      if (mode === 'mutation') invalidateReadCaches()
      if (mode === 'account') localStorage.setItem('auth_token', 'another-user')
      if (mode === 'language') language.current = 'en'
      expect(await read(mode === 'force')).toBe(2)
      old.resolve(1)
      await pending
      expect(await read()).toBe(2)
      expect(load).toHaveBeenCalledTimes(2)
    },
  )
})
