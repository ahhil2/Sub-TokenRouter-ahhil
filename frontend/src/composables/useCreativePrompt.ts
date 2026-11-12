import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { expandCreativePrompt, type CreativeCapabilities } from '@/api/creative'
import { useAuthStore } from '@/stores/auth'
import { getCreativeWorkspaceId } from '@/utils/creativeLocalStore'

/**
 * useCreativePrompt.ts coordinates paid prompt expansion and local restoration
 * for the creation composer. A successful expansion retains the first submitted
 * prompt for undo. An unresolved request keeps its idempotency key until its
 * input changes or a result arrives, allowing a network retry to recover the
 * existing operation. Restoring text changes browser state and makes no API
 * request.
 *
 * A prompt revision identifies intervening edits, including edits that return
 * to the submitted text. Completion updates the composer only while that
 * revision, account, and workspace still match. Unmount cancels the browser
 * request and invalidates publication; the server owns billing completion.
 */

/**
 * useCreativePrompt binds expansion actions to a mounted composer's shared
 * prompt and capability refs. The caller displays the returned error and
 * disables prompt input while busy; external edits are still checked before
 * publishing an asynchronous result. The first successful expansion captures
 * its input for restoration, while unresolved retries reuse their operation key.
 * Account changes clear that snapshot and prevent responses from updating another
 * user's draft.
 */
export function useCreativePrompt(prompt: Ref<string>, capabilities: Ref<CreativeCapabilities>) {
  // Translation and account state belong to the mounted composer and determine
  // which completed requests may update its text and displayed wallet balance.
  const { t } = useI18n()
  const auth = useAuthStore()
  const busy = ref(false)
  const error = ref('')
  const originalPrompt = ref<string | null>(null)
  // Revisions invalidate late results even when later edits recreate the same
  // text. The controller belongs to the current request and is aborted on unmount.
  let revision = 0
  let mounted = true
  let controller: AbortController | null = null
  let pendingInput = ''
  let pendingKey = ''
  // Restoration belongs to the workspace that produced the first expansion;
  // clearing local creative data invalidates its retained input along with retries.
  let originalWorkspace: string | null = null

  watch(
    prompt,
    () => {
      revision += 1
      validateOriginalWorkspace()
    },
    { flush: 'sync' },
  )
  watch(
    () => auth.user?.id,
    () => {
      revision += 1
      clearHistory()
    },
  )

  /** canExpand requires a configured model and a nonempty prompt within its limit. */
  const canExpand = computed(() =>
    Boolean(
      capabilities.value.prompt_expansion_enabled &&
      !busy.value &&
      prompt.value.trim() &&
      Array.from(prompt.value.trim()).length <= capabilities.value.max_prompt_chars,
    ),
  )
  /** canRestore reports whether a successful expansion has recoverable original text. */
  const canRestore = computed(
    () => !busy.value && originalPrompt.value !== null && originalPrompt.value !== prompt.value,
  )

  /**
   * expand requests one expansion and applies it to the matching prompt revision.
   * Calling while unavailable does nothing. Failed requests retain the submitted
   * text and operation key; the first successful replacement captures the undo
   * snapshot and later expansions retain it. Account refresh runs after success
   * and cannot turn a delivered expansion into a failed operation. Results for
   * an edited or discarded workspace do not replace the current draft.
   */
  async function expand(): Promise<void> {
    if (!canExpand.value) return
    busy.value = true
    error.value = ''
    const submittedPrompt = prompt.value
    const submittedRevision = revision
    const userId = auth.user?.id
    try {
      const workspaceId = getCreativeWorkspaceId()
      const input = JSON.stringify([userId, workspaceId, submittedPrompt])
      if (input !== pendingInput || !pendingKey) {
        pendingInput = input
        pendingKey = crypto.randomUUID()
      }
      controller = new AbortController()
      const result = await expandCreativePrompt(
        submittedPrompt,
        workspaceId,
        pendingKey,
        controller.signal,
      )
      if (!mounted || auth.user?.id !== userId) return
      pendingInput = ''
      pendingKey = ''
      // A completed charge changes the wallet even when an external draft edit
      // prevents applying its response, so refresh the balance independently.
      void auth.refreshUser().catch(() => {
        /* Account refresh failure leaves the delivered prompt and charge intact; a later profile request recovers the displayed balance. */
      })
      if (revision !== submittedRevision || getCreativeWorkspaceId() !== workspaceId) {
        error.value = t('creative.prompt.changed')
        return
      }
      if (originalPrompt.value === null) {
        originalPrompt.value = submittedPrompt
        originalWorkspace = workspaceId
      }
      prompt.value = result.prompt
    } catch (cause) {
      if (mounted && auth.user?.id === userId) {
        error.value = (cause as { message?: string })?.message || t('creative.prompt.failed')
      }
    } finally {
      busy.value = false
      controller = null
    }
  }

  /**
   * restore replaces the draft with the first successful expansion's input.
   * It is available after completion and clears the snapshot so a subsequent
   * expansion starts a separate local undo sequence.
   */
  function restore(): void {
    if (!validateOriginalWorkspace() || !canRestore.value || originalPrompt.value === null) return
    prompt.value = originalPrompt.value
    clearHistory()
  }

  /** clearHistory discards saved input and request identity after workspace or account changes. */
  function clearHistory(): void {
    originalPrompt.value = null
    originalWorkspace = null
    pendingInput = ''
    pendingKey = ''
    error.value = ''
  }

  /**
   * validateOriginalWorkspace rejects restoration after local data has been cleared.
   * Storage failure also discards the snapshot because ownership cannot be verified;
   * the current editable prompt remains unchanged in both cases.
   */
  function validateOriginalWorkspace(): boolean {
    if (!originalWorkspace) return true
    try {
      if (getCreativeWorkspaceId() === originalWorkspace) return true
    } catch {
      // Unavailable storage cannot confirm ownership of the retained prompt, so
      // discard its undo state before exposing text in an uncertain workspace.
    }
    clearHistory()
    return false
  }

  // Invalidate publication before cancellation because request completion and
  // component teardown can occur during adjacent asynchronous browser callbacks.
  onBeforeUnmount(() => {
    mounted = false
    controller?.abort()
  })

  return { busy, error, canExpand, canRestore, expand, restore }
}
