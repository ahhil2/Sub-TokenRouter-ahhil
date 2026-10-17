import { describe, expect, it } from 'vitest'

import { getKeyGroupProvider } from '../keyGroupProviders'

const group = (display_brand: string | null | undefined, name = 'custom-group') => ({
  display_brand,
  name,
})

describe('getKeyGroupProvider', () => {
  it('classifies Anthropic and OpenAI by display brand', () => {
    expect(getKeyGroupProvider(group('Anthropic'))).toBe('anthropic')
    expect(getKeyGroupProvider(group('OpenAI'))).toBe('openai')
  })

  it('classifies supported domestic brands', () => {
    for (const brand of ['DeepSeek', 'Moonshot / Kimi', '智谱', '阿里', '百度', '讯飞', '腾讯', '360', '零一', '豆包', '小米', 'MiniMax']) {
      expect(getKeyGroupProvider(group(brand))).toBe('domestic')
    }
  })

  it('falls back to the group name when display brand is empty', () => {
    expect(getKeyGroupProvider(group('', 'OpenAI'))).toBe('openai')
    expect(getKeyGroupProvider(group(null, 'DeepSeek'))).toBe('domestic')
  })

  it('places other and custom brands in the other category', () => {
    expect(getKeyGroupProvider(group('Gemini'))).toBe('other')
    expect(getKeyGroupProvider(group('Grok'))).toBe('other')
    expect(getKeyGroupProvider(group('custom provider'))).toBe('other')
    expect(getKeyGroupProvider(group('Anthropic + DeepSeek'))).toBe('other')
    expect(getKeyGroupProvider(group('Anthropic + Gemini'))).toBe('other')
    expect(getKeyGroupProvider(group('DeepSeek / Grok'))).toBe('other')
  })
})
