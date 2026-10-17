export interface SemanticErrorPolicy {
  enabled: boolean
  texts: string[]
  cooldown_minutes: number
}

export const defaultSemanticErrorText = '[No upstream account is currently available. Please retry later.]'

export function readSemanticErrorPolicy(extra: unknown): SemanticErrorPolicy {
  const policy = (extra as Record<string, unknown> | undefined)?.semantic_error as Partial<SemanticErrorPolicy> | undefined
  return {
    enabled: policy?.enabled === true,
    texts: Array.isArray(policy?.texts) ? policy.texts.filter((text): text is string => typeof text === 'string') : [defaultSemanticErrorText],
    cooldown_minutes: typeof policy?.cooldown_minutes === 'number' ? policy.cooldown_minutes : 5
  }
}

export function isSemanticErrorPolicyValid(policy: SemanticErrorPolicy): boolean {
  if (!policy.enabled) return true
  return Number.isInteger(policy.cooldown_minutes) && policy.cooldown_minutes >= 1 && policy.cooldown_minutes <= 1440 &&
    policy.texts.length >= 1 && policy.texts.length <= 10 && policy.texts.every(text => {
      const length = Array.from(text.trim().replace(/\s+/gu, ' ')).length
      return length >= 8 && length <= 512
    })
}
