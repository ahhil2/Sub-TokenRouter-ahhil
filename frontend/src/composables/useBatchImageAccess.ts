import { computed, ref, watch } from 'vue'
import { keysAPI } from '@/api/keys'
import { useAuthStore } from '@/stores/auth'

/**
 * useBatchImageAccess.ts shares the batch-image menu capability across authenticated
 * consumers. The server answers one existence query for visible active keys, and
 * successful results remain available until refresh or identity change. Failed
 * reads leave readiness false so a later call can retry.
 *
 * One generation owns publication across ordinary shared requests and forced
 * refreshes. Identity changes clear cached permission before starting another read;
 * late results also check the current authenticated user before publishing. Vue
 * consumers expose values only when their current user owns the cache. Scope-owned
 * identity watchers end with their consumers, while the module retains shared state
 * for other mounted navigation components.
 */
// Shared flags and ownership retain one capability observation across menu consumers;
// the generation rejects obsolete completions after identity changes or forced reads.
const loaded = ref(false)
const loading = ref(false)
const allowed = ref(false)
const owner = ref<number | undefined>()
let generation = 0
let pending: Promise<boolean> | null = null

/**
 * loadBatchImageAccess returns the current user's batch-image capability.
 * Ordinary calls share pending work or a successful cached result; force starts a
 * new generation. Unauthenticated calls reset shared state and return false.
 *
 * A completion publishes only when its generation and owner still match the
 * current user. Older promises still resolve to their caller but cannot replace
 * the displayed permission. Failure returns false while keeping readiness unset,
 * allowing retry, and only the owning pending request can clear the loading flag.
 */
async function loadBatchImageAccess(force = false): Promise<boolean> {
  const auth = useAuthStore()
  const userID = auth.isAuthenticated ? auth.user?.id : undefined
  if (owner.value !== userID || !auth.isAuthenticated) {
    // Reset permission before starting another identity's request so mounted navigation
    // cannot display the previous user's capability while the new result is pending.
    owner.value = userID
    generation++
    pending = null
    loaded.value = false
    loading.value = false
    allowed.value = false
  }
  if (!auth.isAuthenticated) return false
  if (pending && !force) return pending
  if (loaded.value && !force) return allowed.value
  const current = ++generation
  loaded.value = false
  loading.value = true
  const request = keysAPI
    .getCapabilities()
    .then((capabilities) => {
      if (current === generation && owner.value === userID && auth.user?.id === userID) {
        allowed.value = capabilities.batch_images
        loaded.value = true
      }
      return capabilities.batch_images
    })
    // Keep readiness unset after failure so another consumer can retry; suppress the
    // request error because this optional navigation hint must not block the page.
    .catch(() => false)
    .finally(() => {
      if (pending === request) {
        loading.value = false
        pending = null
      }
    })
  pending = request
  return request
}

// useBatchImageAccess exposes shared access flags and the refresh operation. Creating a
// consumer observes identity changes; callers explicitly request the initial read and
// choose when a forced refresh is needed.
export function useBatchImageAccess() {
  const auth = useAuthStore()
  watch(
    [() => auth.isAuthenticated, () => auth.user?.id],
    () => {
      if (!auth.isAuthenticated || owner.value !== auth.user?.id) void loadBatchImageAccess()
    },
    { flush: 'sync' },
  )
  const isOwner = computed(() => auth.isAuthenticated && owner.value === auth.user?.id)
  return {
    canUseBatchImage: computed(() => isOwner.value && allowed.value),
    batchImageAccessLoaded: computed(() => isOwner.value && loaded.value),
    batchImageAccessLoading: computed(() => isOwner.value && loading.value),
    refreshBatchImageAccess: loadBatchImageAccess,
  }
}
