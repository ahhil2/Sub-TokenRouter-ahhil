import { describe, expect, it } from 'vitest'
import {
  defaultSemanticErrorText,
  isSemanticErrorPolicyValid,
  readSemanticErrorPolicy,
} from '../semanticErrorPolicy'

/**
 * semanticErrorPolicy.spec.ts checks saved response-text rules, default values, and
 * validation limits. Fixtures distinguish independently copied pattern arrays from
 * persisted input and reject unsupported cooldowns, text counts, and normalized
 * lengths for enabled provider policies.
 */

describe('semanticErrorPolicy', () => {
  it('defaults to disabled with a suggested text and five minute cooldown', () => {
    expect(readSemanticErrorPolicy(undefined)).toEqual({
      enabled: false,
      texts: [defaultSemanticErrorText],
      cooldown_minutes: 5,
    })
  })

  it('loads an independent copy of saved rules', () => {
    const saved = {
      semantic_error: { enabled: true, texts: ['Upstream unavailable'], cooldown_minutes: 10 },
    }
    const policy = readSemanticErrorPolicy(saved)
    expect(policy).toEqual(saved.semantic_error)
    policy.texts.push('Another upstream failure')
    expect(saved.semantic_error.texts).toHaveLength(1)
  })

  it.each([0, -1, 1.5, 1441, Number.NaN])('rejects cooldown %s', (cooldown) => {
    expect(
      isSemanticErrorPolicyValid({
        enabled: true,
        texts: [defaultSemanticErrorText],
        cooldown_minutes: cooldown,
      }),
    ).toBe(false)
  })

  it('checks text counts and normalized length', () => {
    const policy = { enabled: true, texts: [defaultSemanticErrorText], cooldown_minutes: 1 }
    expect(isSemanticErrorPolicyValid(policy)).toBe(true)
    for (const texts of [
      [],
      ['short'],
      [' '.repeat(20)],
      ['x'.repeat(513)],
      Array(11).fill(defaultSemanticErrorText),
    ]) {
      expect(isSemanticErrorPolicyValid({ ...policy, texts })).toBe(false)
    }
  })
})
