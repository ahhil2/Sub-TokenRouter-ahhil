import { getLocale } from '@/i18n'

/**
 * readCache.ts shares configuration reads within a credential and language scope.
 * Each reader owns one successful value and one pending request. Successful values
 * expire after the selected lifetime; every caller receives a structured clone so
 * editing one form cannot mutate another caller's choices or the retained value.
 *
 * Invalidation advances a module revision consulted by every reader. Force refresh
 * also replaces that reader's pending ownership. Earlier callers still receive their
 * own results, but a replaced request cannot publish into the current cache. Failed
 * requests release pending state and can be retried. Credential access failures use
 * an empty credential scope for the current page.
 */
// revision changes after successful API writes so subsequent reads refresh their data.
let revision = 0

/**
 * invalidateReadCaches advances the shared revision used by all configuration readers.
 */
export function invalidateReadCaches(): void {
  revision++
}

/**
 * createCachedRequest returns a reader with a default thirty-second successful cache.
 * The loader must return structured-cloneable data, and each returned value belongs
 * to its caller. Passing true to the reader replaces its cached and pending state.
 *
 * Pending requests are shared within the current revision, language, and credential.
 * Publication checks both request identity and scope, preventing a late response
 * from restoring invalidated data. Rejections propagate and remain retryable; force
 * refresh leaves previously returned promises free to finish for their original callers.
 */
export function createCachedRequest<T>(load: () => Promise<T>, ttl = 30_000) {
  // cached and pending belong to this reader. Scope separates account, language,
  // and invalidation generations; only the pending owner may publish a cached value.
  let cached:
    | {
        scope: string // Credential and revision identity.
        value: T // Loader-owned successful response.
        expires: number // Epoch milliseconds expiry.
      }
    | undefined
  let pending:
    | {
        scope: string // Request's originating scope.
        promise: Promise<T> // Shared loader completion.
      }
    | undefined

  /**
   * scope identifies the current revision, locale, and browser credential for reuse.
   * Unavailable browser storage contributes an empty credential. Callers compare this
   * value again before publishing a completed request into the shared reader cache.
   */
  function scope(): string {
    let token = ''
    try {
      token = globalThis.localStorage?.getItem('auth_token') ?? ''
    } catch {
      // Storage can be disabled by browser policy; use the page scope while revision
      // and language still distinguish invalidated or translated configuration.
    }
    return `${revision}:${getLocale()}:${token}`
  }

  return async (force = false): Promise<T> => {
    // A forced read replaces ownership before loading, so an older completion cannot
    // repopulate the cache even when its original caller still awaits that response.
    if (force) {
      cached = undefined
      pending = undefined
    }
    const key = scope()
    if (cached?.scope === key && cached.expires > Date.now()) {
      return structuredClone(cached.value)
    }
    if (!pending || pending.scope !== key) {
      const request = { scope: key, promise: Promise.resolve().then(load) }
      pending = request
      request.promise = request.promise
        .then((value) => {
          // Writes and identity changes can happen during loading. Recheck both
          // ownership and current scope before retaining data for another consumer.
          if (pending === request && scope() === key) {
            cached = { scope: key, value, expires: Date.now() + ttl }
          }
          return value
        })
        .finally(() => {
          if (pending === request) pending = undefined
        })
    }
    return structuredClone(await pending.promise)
  }
}
