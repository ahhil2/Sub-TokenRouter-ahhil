// register.jsx renders the SuperAHHil registration page and creates accounts through
// the same-origin TokenRouter API. It reads public status for registration, email
// verification and Turnstile settings, hands off to /email-verify when email
// verification is enabled, and otherwise redirects to the dashboard after signup.
// React, ReactDOM and lucide-react come from pinned CDN globals declared in
// ../site/register.html; build.mjs compiles this file into that page.

const { useEffect, useRef, useState } = React;
const { createRoot } = ReactDOM;
const {
  ArrowRight,
  CheckCircle2,
  Eye,
  EyeOff,
  Loader2,
} = LucideReact;

const defaultStatus = {
  system_name: "SuperAHHilAi",
  register_enabled: true,
  password_register_enabled: true,
  email_verification: true,
  turnstile_check: false,
  turnstile_site_key: "",
  user_agreement_enabled: false,
  privacy_policy_enabled: false,
  agreement_documents: [],
  invitation_code_enabled: false,
  promo_code_enabled: false,
};

async function readApiResponse(response) {
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    throw new Error((payload && payload.message) || `请求失败 (${response.status})`);
  }
  if (payload && typeof payload.code === "number") {
    if (payload.code !== 0) {
      throw new Error(payload.message || "请求失败");
    }
    return payload.data;
  }
  return payload;
}

function persistAuthSession(data) {
  if (!data || !data.access_token || !data.user) {
    throw new Error("注册响应缺少认证信息");
  }
  const { run_mode, ...user } = data.user;
  localStorage.setItem("auth_token", data.access_token);
  localStorage.setItem("auth_user", JSON.stringify(user));
  if (data.refresh_token) {
    localStorage.setItem("refresh_token", data.refresh_token);
  } else {
    localStorage.removeItem("refresh_token");
  }
  if (data.expires_in) {
    localStorage.setItem("token_expires_at", String(Date.now() + data.expires_in * 1000));
  } else {
    localStorage.removeItem("token_expires_at");
  }
  localStorage.removeItem("pending_auth_session");
}

function App() {
  const [status, setStatus] = useState(defaultStatus);
  const [loadingStatus, setLoadingStatus] = useState(true);
  const [toast, setToast] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [accepted, setAccepted] = useState(false);
  const [turnstileToken, setTurnstileToken] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);
  const [form, setForm] = useState({
    email: "",
    password: "",
    password2: "",
    aff_code: "",
    invitation_code: "",
    promo_code: "",
  });
  const turnstileRef = useRef(null);
  const widgetRef = useRef(null);

  const needsAgreement = status.user_agreement_enabled || status.privacy_policy_enabled;

  useEffect(() => {
    const aff = new URLSearchParams(window.location.search).get("aff") || localStorage.getItem("aff") || "";
    if (aff) {
      localStorage.setItem("aff", aff);
      setForm((current) => ({ ...current, aff_code: aff }));
    }

    fetch("/api/v1/settings/public", { credentials: "same-origin" })
      .then(readApiResponse)
      .then((settings) => {
        setStatus({
          ...defaultStatus,
          system_name: settings.site_name || defaultStatus.system_name,
          register_enabled: settings.registration_enabled !== false,
          email_verification: Boolean(settings.email_verify_enabled),
          turnstile_check: Boolean(settings.turnstile_enabled),
          turnstile_site_key: settings.turnstile_site_key || "",
          user_agreement_enabled: Boolean(settings.login_agreement_enabled),
          agreement_documents: settings.login_agreement_documents || [],
          invitation_code_enabled: Boolean(settings.invitation_code_enabled),
          promo_code_enabled: Boolean(settings.promo_code_enabled),
        });
      })
      .catch((error) => showToast(error.message || "无法读取 Sub2API 站点配置", "error"))
      .finally(() => setLoadingStatus(false));
  }, []);

  useEffect(() => {
    if (!status.turnstile_check || !status.turnstile_site_key) return;
    const renderTurnstile = () => {
      if (!window.turnstile || !turnstileRef.current || widgetRef.current) return;
      widgetRef.current = window.turnstile.render(turnstileRef.current, {
        sitekey: status.turnstile_site_key,
        theme: "dark",
        callback: (token) => setTurnstileToken(token),
        "expired-callback": () => setTurnstileToken(""),
        "error-callback": () => setTurnstileToken(""),
      });
    };
    renderTurnstile();
    window.addEventListener("turnstile-ready", renderTurnstile);
    return () => window.removeEventListener("turnstile-ready", renderTurnstile);
  }, [status.turnstile_check, status.turnstile_site_key]);

  function showToast(message, type = "success") {
    setToast({ message, type });
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(() => setToast(null), 3600);
  }

  function updateField(name, value) {
    setForm((current) => ({ ...current, [name]: value }));
  }

  function requireTurnstile() {
    if (status.turnstile_check && !turnstileToken) {
      showToast("请稍后几秒重试，Turnstile 正在检查用户环境。", "error");
      return false;
    }
    return true;
  }

  async function submitRegister(event) {
    event.preventDefault();
    if (!status.register_enabled || !status.password_register_enabled) {
      showToast("当前站点未开放账号密码注册。", "error");
      return;
    }
    if (needsAgreement && !accepted) {
      showToast("请先阅读并同意用户协议和隐私政策。", "error");
      return;
    }
    if (!form.email.trim()) {
      showToast("请填写邮箱。", "error");
      return;
    }
    if (status.invitation_code_enabled && !form.invitation_code.trim()) {
      showToast("请输入邀请码。", "error");
      return;
    }
    if (form.password.length < 6) {
      showToast("密码长度不得小于 6 位。", "error");
      return;
    }
    if (form.password !== form.password2) {
      showToast("两次输入的密码不一致。", "error");
      return;
    }
    if (!requireTurnstile()) return;

    const affCode =
      form.aff_code.trim() ||
      new URLSearchParams(window.location.search).get("aff") ||
      localStorage.getItem("aff") ||
      "";

    const registrationData = {
      email: form.email.trim(),
      password: form.password,
      turnstile_token: status.turnstile_check ? turnstileToken : undefined,
      promo_code: form.promo_code.trim() || undefined,
      invitation_code: form.invitation_code.trim() || undefined,
      aff_code: affCode || undefined,
    };

    if (status.email_verification) {
      sessionStorage.setItem("register_data", JSON.stringify(registrationData));
      window.location.href = "/email-verify";
      return;
    }

    setSubmitting(true);
    try {
      const res = await fetch("/api/v1/auth/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(registrationData),
      });
      const data = await readApiResponse(res);
      persistAuthSession(data);
      localStorage.removeItem("aff");
      showToast("注册成功，正在进入控制台。");
      setTimeout(() => { window.location.href = "/dashboard"; }, 500);
    } catch (error) {
      showToast(error.message || "注册失败，请重试。", "error");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="app">
      <div className="pyramid"></div>
      <div className="wrap">
        <a className="brand fade-up delay-1" href="/">
          <span className="brand-mark">I</span>
          <span>
            <span className="brand-title">{status.system_name || "SuperAHHilAi"}</span>
            <span className="brand-subtitle">AI API relay console</span>
          </span>
        </a>
        <a className="home-link fade-up delay-1" href="/">返回首页</a>

        <section className="hero fade-up delay-2">
          <div className="pill">
            <span className="pill-dot"></span>
            新用户进群发 id 送额度
          </div>
          <h1>创建你的 AI 工具中转账户</h1>
          <p>
            一个账户统一接入 Claude Code、Codex、Gemini CLI 与图像模型。注册后在控制台创建密钥，按需查看额度、日志与模型状态。
          </p>
          <div className="metrics">
            <div className="metric">
              <strong>$2</strong>
              <span>进群额度</span>
            </div>
            <div className="metric">
              <strong>统一</strong>
              <span>密钥与账单</span>
            </div>
            <div className="metric">
              <strong>实时</strong>
              <span>额度日志</span>
            </div>
          </div>
        </section>

        <section className="form-card fade-up delay-3">
          <div className="form-head">
            <div>
              <h2>创建新账户</h2>
              <p>输入邮箱并设置密码</p>
            </div>
          </div>

          <div className="tabs">
            <a className="tab" href="/login">登录</a>
            <button className="tab tab-active" type="button">注册</button>
          </div>

          <form className="form" onSubmit={submitRegister}>
            <Field
              label="邮箱"
              type="email"
              placeholder="you@example.com"
              value={form.email}
              onChange={(value) => updateField("email", value)}
              disabled={loadingStatus || !status.register_enabled}
            />

            <PasswordField
              label="密码"
              placeholder="至少 8 位字符"
              value={form.password}
              show={showPassword}
              onToggle={() => setShowPassword((value) => !value)}
              onChange={(value) => updateField("password", value)}
            />

            <PasswordField
              label="确认密码"
              placeholder="再次输入密码"
              value={form.password2}
              show={showConfirm}
              onToggle={() => setShowConfirm((value) => !value)}
              onChange={(value) => updateField("password2", value)}
            />

            {status.invitation_code_enabled && (
              <Field
                label="邀请码"
                placeholder="输入管理员提供的邀请码"
                value={form.invitation_code}
                onChange={(value) => updateField("invitation_code", value)}
              />
            )}

            {status.promo_code_enabled && (
              <Field
                label="优惠码（选填）"
                placeholder="输入优惠码"
                value={form.promo_code}
                onChange={(value) => updateField("promo_code", value)}
              />
            )}

            <Field
              label="推荐码（选填）"
              placeholder="邀请链接会自动填写"
              value={form.aff_code}
              onChange={(value) => updateField("aff_code", value)}
            />

            {status.turnstile_check && (
              <div className="field">
                <label>安全验证</label>
                <div ref={turnstileRef}></div>
              </div>
            )}

            {needsAgreement && (
              <label className="agreement">
                <input
                  type="checkbox"
                  checked={accepted}
                  onChange={(event) => setAccepted(event.target.checked)}
                />
                <span>
                  我已阅读并同意
                  {(status.agreement_documents || []).map((document) => (
                    <a key={document.id} href={`/legal/${encodeURIComponent(document.id)}`} target="_blank"> {document.title} </a>
                  ))}
                </span>
              </label>
            )}

            <button
              className="primary-btn"
              type="submit"
              disabled={submitting || loadingStatus || !status.register_enabled}
            >
              {submitting ? (
                <>
                  <Loader2 className="spin" size={20} />
                  正在创建账户
                </>
              ) : (
                <>
                  创建账户
                  <ArrowRight size={20} />
                </>
              )}
            </button>
          </form>

          <p className="footnote">
            注册即表示你同意合理使用平台资源；服务状态与模型可用性以控制台实时显示为准。
          </p>
        </section>
      </div>

      {toast && (
        <div className={`toast ${toast.type === "error" ? "toast-error" : ""}`}>
          {toast.message}
        </div>
      )}
    </main>
  );
}

function Field({ label, placeholder, type = "text", value, onChange, disabled = false }) {
  return (
    <div className="field">
      <label>{label}</label>
      <input
        className="input"
        type={type}
        placeholder={placeholder}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  );
}

function PasswordField({ label, placeholder, value, show, onToggle, onChange }) {
  return (
    <div className="field password">
      <label>{label}</label>
      <div className="input-wrap">
        <input
          className="input"
          type={show ? "text" : "password"}
          placeholder={placeholder}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
        <button className="eye-btn" type="button" onClick={onToggle} aria-label={show ? "隐藏密码" : "显示密码"}>
          {show ? <EyeOff size={20} /> : <Eye size={20} />}
        </button>
      </div>
    </div>
  );
}

createRoot(document.getElementById("root")).render(<App />);
