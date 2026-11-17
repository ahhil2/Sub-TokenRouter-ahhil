// shared.ts supplies administrator-namespaced text used by shared provider badges and
// payment error formatting. Ordinary pages can display these labels before an
// administrator opens a configuration page. Full domain dictionaries reuse the same
// entries so route loading and language changes resolve identical wording.

// providerStatusMessages labels provider state and privacy results in shared badges.
export const providerStatusMessages = {
  privacyTrainingOff: '已关闭训练数据共享',
  privacyCfBlocked: '被 Cloudflare 拦截，训练可能仍开启',
  privacyFailed: '关闭训练数据共享失败',
  privacyAntigravitySet: '已关闭遥测和营销邮件',
  privacyAntigravityFailed: '隐私设置失败',
  subscriptionAbnormal: '异常',
  subscriptionExpires: '到期',
}

// paymentFieldMessages names payment configuration fields when formatted errors identify
// the rejected keys. Both public payment failures and administrator forms use these labels.
export const paymentFieldMessages = {
  field_epusdtApiBaseHint:
    '填写 EPUSDT HTTPS 基础地址，例如 https://pay.example.com。使用 EPay submit.php 跳转协议和 MD5 签名，密钥填写商户 API Key 的 secret。',
  field_apiBase: 'API 基础地址',
  field_notifyUrl: '异步通知地址',
  field_returnUrl: '同步跳转地址',
  field_privateKey: '私钥',
  field_publicKey: '公钥',
  field_mpAppId: '公众号 App ID',
  field_mchId: '商户号',
  field_apiV3Key: 'API v3 密钥',
  field_publicKeyId: '公钥 ID',
  field_certSerial: '证书序列号',
  field_h5AppName: 'H5 应用名称',
  field_h5AppUrl: 'H5 应用地址',
  field_secretKey: '密钥',
  field_clientId: 'Client ID',
  field_apiKey: 'API Key',
  field_publishableKey: '公开密钥',
  field_webhookSecret: 'Webhook 密钥',
  field_countryCode: '国家/地区代码',
  field_currency: '支付币种',
  field_accountId: 'Airwallex 账户 ID',
  field_airwallexApiBaseHint:
    '必须和 API Key 所属环境一致：沙箱/测试密钥使用 https://api-demo.airwallex.com/api/v1，生产密钥使用 https://api.airwallex.com/api/v1。环境混用会返回 credentials_invalid / Access Denied。',
  field_paymentCurrencyHint:
    '默认 CNY。Stripe 和 Airwallex 可按账户支持从下拉项选择 HKD、USD 等币种；微信、支付宝、易支付仍按 CNY。',
  field_accountIdHint:
    '不涉及多账户、组织级密钥或连接账户收款时可以不填；单账户 Scoped API Key 会默认使用所选账户。',
  field_cid: '支付渠道 ID',
  field_cidAlipay: '支付宝渠道 ID',
  field_cidWxpay: '微信渠道 ID',
}
