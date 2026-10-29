import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Component } from 'vue'
import RegisterView from '@/views/auth/RegisterView.vue'
import EmailVerifyView from '@/views/auth/EmailVerifyView.vue'
import { useAppStore } from '@/stores/app'
import { getPublicSettings, sendVerifyCode } from '@/api/auth'
import type { PublicSettings } from '@/api/settings.types'
import { i18n } from '@/i18n'
import { localizedErrorMessage } from '@/i18n/errors'

// RegistrationSettings.spec.ts exercises signup and email verification against
// the real application settings store. Controlled API responses reproduce cached
// configuration, overlapping reads, and unavailable policy while mounted forms
// expose whether account creation or email delivery can escape the loading gate.

// Navigation and signup remain observable substitutes while configuration uses
// the production store, including its injected settings and pending-request reuse.
const { push, register } = vi.hoisted(() => ({ push: vi.fn(), register: vi.fn() }))
const wrappers: VueWrapper[] = [] // mounted views awaiting cleanup

vi.mock('vue-router', () => ({
  // useRouter records the destination chosen after a signup form submission.
  useRouter: () => ({ push }),
  // useRoute supplies empty referral and promotion queries for these scenarios.
  useRoute: () => ({ query: {} }),
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  // useI18n exposes message keys and site-name interpolation to component assertions.
  useI18n: () => ({
    // t keeps configured branding observable without the runtime-only message compiler.
    t: (key: string, values?: Record<string, unknown>) =>
      values?.siteName ? `${key}: ${values.siteName}` : key,
    locale: { value: 'en' },
  }),
}))
vi.mock('@/stores', async () => {
  const { useAppStore } = await import('@/stores/app')
  return {
    useAppStore,
    // useAuthStore records ordinary registration without creating a real session.
    useAuthStore: () => ({ register, pendingAuthSession: null }),
  }
})
vi.mock('@/api/auth', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/auth')>()),
  getPublicSettings: vi.fn(),
  sendVerifyCode: vi.fn(),
}))
vi.mock('@/composables/useBalanceDisplay', () => ({
  // useBalanceDisplay supplies an inert balance formatter for hidden promotion UI.
  useBalanceDisplay: () => ({ formatBalanceAmount: String }),
}))

// settings builds the policy fields consumed by these two mounted views. Callers
// override individual requirements to exercise verified and direct registration;
// unrelated public settings fields are unused by the fixture.
function settings(overrides: Partial<PublicSettings> = {}): PublicSettings {
  return {
    locale: 'en',
    registration_enabled: true,
    email_verify_enabled: true,
    promo_code_enabled: false,
    invitation_code_enabled: false,
    affiliate_enabled: false,
    turnstile_enabled: false,
    login_agreement_enabled: false,
    site_name: 'Configured Site',
    registration_email_suffix_whitelist: [],
    ...overrides,
  } as PublicSettings
}

// mountView renders an auth view with the real settings store. External
// provider widgets and layout are stubbed, and teardown owns every wrapper so
// verification countdowns are released after each case.
function mountView(component: Component) {
  const wrapper = mount(component, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        EmailOAuthButtons: true,
        LinuxDoOAuthSection: true,
        WechatOAuthSection: true,
        OidcOAuthSection: true,
        TurnstileWidget: true,
        Icon: true,
        RouterLink: { template: '<a><slot /></a>' },
      },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

// fillRegistration enters valid credentials through the mounted signup form inputs.
async function fillRegistration(wrapper: VueWrapper) {
  await wrapper.get('#email').setValue('alice@example.com')
  await wrapper.get('#password').setValue('secret123')
}

// saveDraft supplies the browser continuation consumed by the email verification
// view. Credentials remain in session storage until registration completes, so
// failed settings reads can be retried without losing them.
function saveDraft() {
  sessionStorage.setItem(
    'register_data',
    JSON.stringify({ email: 'alice@example.com', password: 'secret123' }),
  )
}

describe('registration settings availability', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    i18n.global.locale.value = 'en'
    delete window.__APP_CONFIG__
    localStorage.clear()
    sessionStorage.clear()
    vi.mocked(getPublicSettings).mockReset().mockResolvedValue(settings())
    vi.mocked(sendVerifyCode).mockReset().mockResolvedValue({ countdown: 60 })
    register.mockReset().mockResolvedValue(undefined)
    push.mockReset().mockResolvedValue(undefined)
  })

  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
    vi.restoreAllMocks()
    delete window.__APP_CONFIG__
  })

  // HTML already contains the required verification policy and branding. Signup
  // must reach verification, and both pages must reuse that configuration without
  // downloading the public settings endpoint again.
  it('reuses injected configuration throughout verified signup', async () => {
    window.__APP_CONFIG__ = settings()
    const signup = mountView(RegisterView)
    await flushPromises()
    expect(signup.text()).toContain('Configured Site')
    await fillRegistration(signup)
    await signup.get('form').trigger('submit')
    await flushPromises()
    expect(push).toHaveBeenCalledWith('/email-verify')
    expect(register).not.toHaveBeenCalled()
    const verification = mountView(EmailVerifyView)
    await flushPromises()
    expect(getPublicSettings).not.toHaveBeenCalled()
    expect(sendVerifyCode).toHaveBeenCalledTimes(1)
    await verification.get('#code').setValue('123456')
    await verification.get('form').trigger('submit')
    await flushPromises()
    expect(register).toHaveBeenCalledWith(expect.objectContaining({ verify_code: '123456' }))
  })

  // Application startup has already fetched configuration for the active locale.
  // Mounting the signup view must preserve that policy and avoid a duplicate read.
  it('reuses a completed application settings request', async () => {
    await useAppStore().fetchPublicSettings()
    const signup = mountView(RegisterView)
    await flushPromises()
    expect(getPublicSettings).toHaveBeenCalledTimes(1)
    await fillRegistration(signup)
    await signup.get('form').trigger('submit')
    await flushPromises()
    expect(push).toHaveBeenCalledWith('/email-verify')
    expect(register).not.toHaveBeenCalled()
  })

  // A slow application request overlaps both auth views. Neither programmatic
  // submit nor resend may start work before that one shared response establishes
  // verification and captcha policy for the views.
  it('joins an active request and blocks actions until it completes', async () => {
    let resolve!: (value: PublicSettings) => void
    vi.mocked(getPublicSettings).mockReturnValueOnce(
      new Promise((done) => {
        resolve = done
      }),
    )
    const pending = useAppStore().fetchPublicSettings()
    saveDraft()
    const signup = mountView(RegisterView)
    const verification = mountView(EmailVerifyView)
    await flushPromises()
    await fillRegistration(signup)
    await signup.get('form').trigger('submit')
    await verification.get('#code').setValue('123456')
    await verification.get('form').trigger('submit')
    expect(verification.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    expect(signup.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    expect(register).not.toHaveBeenCalled()
    expect(sendVerifyCode).not.toHaveBeenCalled()
    expect(getPublicSettings).toHaveBeenCalledTimes(1)
    resolve(settings())
    await pending
    await flushPromises()
    expect(sendVerifyCode).toHaveBeenCalledTimes(1)
    await fillRegistration(signup)
    await signup.get('form').trigger('submit')
    await flushPromises()
    expect(push).toHaveBeenCalledWith('/email-verify')
    expect(register).not.toHaveBeenCalled()
  })

  // A timed-out settings read leaves signup policy unknown. Submitting the form
  // stays blocked, and retrying restores verification before the next submission
  // can proceed with the user's credentials to the verification page.
  it('blocks signup after timeout and recovers through retry', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(getPublicSettings).mockRejectedValueOnce(new Error('timeout of 30000ms exceeded'))
    const signup = mountView(RegisterView)
    await flushPromises()
    await fillRegistration(signup)
    await signup.get('form').trigger('submit')
    expect(register).not.toHaveBeenCalled()
    expect(push).not.toHaveBeenCalled()
    expect(signup.get('[role="alert"]').text()).toContain('auth.settingsLoadFailed')
    expect(signup.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await signup.get('[data-testid="retry-settings"]').trigger('click')
    await flushPromises()
    expect(signup.find('[role="alert"]').exists()).toBe(false)
    await fillRegistration(signup)
    await signup.get('form').trigger('submit')
    await flushPromises()
    expect(getPublicSettings).toHaveBeenCalledTimes(2)
    expect(push).toHaveBeenCalledWith('/email-verify')
    expect(register).not.toHaveBeenCalled()
  })

  // Verification restores credentials before a failed policy read. Delivery,
  // resend, and account submission stay unavailable until retry succeeds, after
  // which the initial verification message is sent once.
  it('blocks verification and delivery on failure, then resumes after retry', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(getPublicSettings).mockRejectedValueOnce(new Error('network interrupted'))
    saveDraft()
    const draft = sessionStorage.getItem('register_data')
    const verification = mountView(EmailVerifyView)
    await flushPromises()
    await verification.get('#code').setValue('123456')
    await verification.get('form').trigger('submit')
    expect(register).not.toHaveBeenCalled()
    expect(sendVerifyCode).not.toHaveBeenCalled()
    expect(verification.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    const resend = verification
      .findAll('button')
      .find((button) => button.text() === 'auth.resendCode')
    expect(resend?.attributes('disabled')).toBeDefined()
    expect(sessionStorage.getItem('register_data')).toBe(draft)
    await verification.get('[data-testid="retry-settings"]').trigger('click')
    await flushPromises()
    expect(getPublicSettings).toHaveBeenCalledTimes(2)
    expect(sendVerifyCode).toHaveBeenCalledTimes(1)
    await verification.get('form').trigger('submit')
    await flushPromises()
    expect(register).toHaveBeenCalledWith(expect.objectContaining({ verify_code: '123456' }))
  })

  // An explicit policy disables email verification for local signup. Successful
  // settings loading must still allow that supported direct-registration path
  // rather than forcing every user through the verification screen.
  it('allows direct signup when loaded policy disables verification', async () => {
    window.__APP_CONFIG__ = settings({ email_verify_enabled: false })
    const signup = mountView(RegisterView)
    await flushPromises()
    await fillRegistration(signup)
    await signup.get('form').trigger('submit')
    await flushPromises()
    expect(register).toHaveBeenCalledTimes(1)
    expect(push).not.toHaveBeenCalledWith('/email-verify')
  })

  // A refresh fails after branding was successfully published by the application.
  // The signup view must retain that displayed name while blocking actions behind
  // its settings error, preventing a misleading default brand during failure.
  it('retains published branding when a shared refresh fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await useAppStore().fetchPublicSettings()
    let reject!: (error: Error) => void
    vi.mocked(getPublicSettings).mockReturnValueOnce(
      new Promise((_resolve, fail) => {
        reject = fail
      }),
    )
    const pending = useAppStore().fetchPublicSettings(true)
    const signup = mountView(RegisterView)
    expect(signup.text()).toContain('Configured Site')
    reject(new Error('timeout'))
    await pending
    await flushPromises()
    expect(signup.text()).toContain('Configured Site')
    expect(signup.get('[role="alert"]').exists()).toBe(true)
    expect(register).not.toHaveBeenCalled()
  })

  // The backend reason identifies missing email verification independently of
  // HTTP 400. Both supported dictionaries must explain the required action rather
  // than replacing it with the generic invalid-parameters message.
  it.each([
    ['en', 'Complete email verification before continuing.'],
    ['zh', '请先完成邮箱验证后再继续。'],
  ] as const)('explains required verification in %s', (language, expected) => {
    i18n.global.locale.value = language
    expect(localizedErrorMessage('EMAIL_VERIFY_REQUIRED', 400)).toBe(expected)
  })
})
