import { effectScope, reactive } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { useBatchImageAccess } from '../useBatchImageAccess'

/**
 * useBatchImageAccess.spec.ts verifies shared capability reads, retry, and authenticated ownership of menu visibility.
 * Controlled requests and browser substitutes isolate the observable state transitions
 * so fixtures can reproduce delayed completion and cleanup without external services.
 */

const getCapabilities = vi.hoisted(() => vi.fn())
const auth = reactive({ isAuthenticated: false, user: { id: 1 } })
vi.mock('@/api/keys', () => ({ keysAPI: { getCapabilities } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
let scope: ReturnType<typeof effectScope>
let access: ReturnType<typeof useBatchImageAccess>

beforeEach(async () => {
  scope = effectScope()
  access = scope.run(useBatchImageAccess)!
  auth.isAuthenticated = false
  await access.refreshBatchImageAccess()
  getCapabilities.mockReset()
  getCapabilities.mockResolvedValue({ batch_images: false })
  auth.user.id = 1
})
afterEach(() => scope.stop())

describe('批量图片菜单资格', () => {
  // Two navigation consumers request the same capability before its response arrives. They
  // share the request, and a later failed refresh remains retryable.
  it('多个入口共用资格请求，失败后可以重试', async () => {
    let resolve!: (value: unknown) => void
    getCapabilities.mockReturnValueOnce(
      new Promise((yes) => {
        resolve = yes
      }),
    )
    auth.isAuthenticated = true
    const first = access.refreshBatchImageAccess()
    const second = access.refreshBatchImageAccess()
    expect(getCapabilities).toHaveBeenCalledOnce()
    resolve({ batch_images: true })
    await Promise.all([first, second])
    expect(access.canUseBatchImage.value).toBe(true)
    getCapabilities.mockRejectedValueOnce(new Error('offline'))
    await access.refreshBatchImageAccess(true)
    expect(access.batchImageAccessLoaded.value).toBe(false)
    await access.refreshBatchImageAccess()
    expect(access.canUseBatchImage.value).toBe(false)
  })

  // An account change occurs while the previous capability response is pending, followed by
  // logout. Old permission must not become visible for another user or guest.
  it('切换账号或退出后丢弃旧资格', async () => {
    let resolve!: (value: unknown) => void
    getCapabilities.mockReturnValueOnce(
      new Promise((yes) => {
        resolve = yes
      }),
    )
    auth.isAuthenticated = true
    const old = access.refreshBatchImageAccess()
    auth.user.id = 2
    await access.refreshBatchImageAccess()
    resolve({ batch_images: true })
    await old
    expect(access.canUseBatchImage.value).toBe(false)
    getCapabilities.mockResolvedValue({ batch_images: true })
    await access.refreshBatchImageAccess(true)
    expect(access.canUseBatchImage.value).toBe(true)
    auth.isAuthenticated = false
    expect(access.canUseBatchImage.value).toBe(false)
    getCapabilities.mockResolvedValue({ batch_images: false })
    auth.isAuthenticated = true
    await flushPromises()
    expect(access.canUseBatchImage.value).toBe(false)
  })
})
