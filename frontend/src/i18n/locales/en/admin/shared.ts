// shared.ts supplies administrator-namespaced text used by shared provider badges and
// payment error formatting. Ordinary pages can display these labels before an
// administrator opens a configuration page. Full domain dictionaries reuse the same
// entries so route loading and language changes resolve identical wording.

// providerStatusMessages labels provider state and privacy results in shared badges.
export const providerStatusMessages = {
  privacyTrainingOff: 'Training data sharing disabled',
  privacyCfBlocked: 'Blocked by Cloudflare, training may still be on',
  privacyFailed: 'Failed to disable training',
  privacyAntigravitySet: 'Telemetry and marketing emails disabled',
  privacyAntigravityFailed: 'Privacy setting failed',
  subscriptionAbnormal: 'Abnormal',
  subscriptionExpires: 'Expires',
}

// paymentFieldMessages names payment configuration fields when formatted errors identify
// the rejected keys. Both public payment failures and administrator forms use these labels.
export const paymentFieldMessages = {
  field_epusdtApiBaseHint:
    'Enter the EPUSDT HTTPS base URL, such as https://pay.example.com. Uses the EPay submit.php flow with MD5 signatures and the merchant API key secret.',
  field_apiBase: 'API Base URL',
  field_notifyUrl: 'Notify URL',
  field_returnUrl: 'Return URL',
  field_privateKey: 'Private Key',
  field_publicKey: 'Public Key',
  field_mpAppId: 'MP App ID',
  field_mchId: 'Merchant ID',
  field_apiV3Key: 'API v3 Key',
  field_publicKeyId: 'Public Key ID',
  field_certSerial: 'Certificate Serial',
  field_h5AppName: 'H5 App Name',
  field_h5AppUrl: 'H5 App URL',
  field_secretKey: 'Secret Key',
  field_clientId: 'Client ID',
  field_apiKey: 'API Key',
  field_publishableKey: 'Publishable Key',
  field_webhookSecret: 'Webhook Secret',
  field_countryCode: 'Country/region code',
  field_currency: 'Payment currency',
  field_accountId: 'Airwallex Account ID',
  field_airwallexApiBaseHint:
    'Must match the API key environment: use https://api-demo.airwallex.com/api/v1 for sandbox/demo keys, and https://api.airwallex.com/api/v1 for production keys. Mixed environments return credentials_invalid / Access Denied.',
  field_paymentCurrencyHint:
    'Default is CNY. Stripe and Airwallex can choose HKD, USD, or another listed currency supported by the account; WeChat Pay, Alipay, and EasyPay remain CNY.',
  field_accountIdHint:
    'Leave this empty unless you use multiple accounts, an organization-level key, or connected-account payments. A single-account scoped API key uses the selected account by default.',
  field_cid: 'Channel ID',
  field_cidAlipay: 'Alipay Channel ID',
  field_cidWxpay: 'WeChat Channel ID',
}
