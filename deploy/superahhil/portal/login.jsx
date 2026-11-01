// login.jsx renders the SuperAHHil password login page and signs users in through the
// same-origin TokenRouter API. It reads public status to decide whether password login,
// registration and Cloudflare Turnstile are enabled, then redirects to the dashboard
// after a successful sign-in. React, ReactDOM and lucide-react come from pinned CDN
// globals declared in ../site/login.html; build.mjs compiles this file into that page.

const { useEffect, useRef, useState } = React;
const { createRoot } = ReactDOM;
const { ArrowRight, Eye, EyeOff, Loader2 } = LucideReact;

const defaultStatus = {
  system_name: "SuperAHHilAi",
  password_login_enabled: true,
  register_enabled: true,
  turnstile_check: false,
  turnstile_site_key: "",
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
    throw new Error("登录响应缺少认证信息");
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
  const [turnstileToken, setTurnstileToken] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [form, setForm] = useState({ email: "", password: "" });
  const [twoFactor, setTwoFactor] = useState({ required: false, tempToken: "", code: "" });
  const turnstileRef = useRef(null);
  const widgetRef = useRef(null);

  useEffect(() => {
    fetch("/api/v1/settings/public", { credentials: "same-origin" })
      .then(readApiResponse)
      .then((settings) => {
        setStatus({
          ...defaultStatus,
          system_name: settings.site_name || defaultStatus.system_name,
          register_enabled: settings.registration_enabled !== false,
          turnstile_check: Boolean(settings.turnstile_enabled),
          turnstile_site_key: settings.turnstile_site_key || "",
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

  async function submitLogin(event) {
    event.preventDefault();

    if (!status.password_login_enabled) {
      showToast("当前站点未开放账号密码登录。", "error");
      return;
    }
    if (!form.email.trim() || !form.password) {
      showToast("请输入邮箱与密码。", "error");
      return;
    }
    if (!requireTurnstile()) return;

    setSubmitting(true);
    try {
      const res = await fetch("/api/v1/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({
          email: form.email.trim(),
          password: form.password,
          turnstile_token: status.turnstile_check ? turnstileToken : undefined,
        }),
      });
      const data = await readApiResponse(res);
      if (data && data.requires_2fa) {
        setTwoFactor({ required: true, tempToken: data.temp_token || "", code: "" });
        showToast("请输入身份验证器中的 6 位验证码。");
        return;
      }
      persistAuthSession(data);
      showToast("登录成功，正在进入控制台。");
      setTimeout(() => { window.location.href = "/dashboard"; }, 500);
    } catch (error) {
      showToast(error.message || "登录失败，请重试。", "error");
    } finally {
      setSubmitting(false);
    }
  }

  async function submitTwoFactor(event) {
    event.preventDefault();
    if (!/^\d{6}$/.test(twoFactor.code.trim())) {
      showToast("请输入 6 位验证码。", "error");
      return;
    }
    setSubmitting(true);
    try {
      const res = await fetch("/api/v1/auth/login/2fa", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({
          temp_token: twoFactor.tempToken,
          totp_code: twoFactor.code.trim(),
        }),
      });
      const data = await readApiResponse(res);
      persistAuthSession(data);
      showToast("验证成功，正在进入控制台。");
      setTimeout(() => { window.location.href = "/dashboard"; }, 500);
    } catch (error) {
      showToast(error.message || "两步验证失败，请重试。", "error");
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
            统一账户 · 控制台入口
          </div>
          <h1>回到你的 AI 工具中转账户</h1>
          <p>
            登录后创建密钥、查看额度与请求日志，把 Claude Code、Codex、Gemini CLI 与图像模型统一接到同一套账户。
          </p>
          <div className="metrics">
            <div className="metric">
              <strong>统一</strong>
              <span>账户与密钥</span>
            </div>
            <div className="metric">
              <strong>实时</strong>
              <span>额度日志</span>
            </div>
            <div className="metric">
              <strong>稳定</strong>
              <span>模型状态</span>
            </div>
          </div>
        </section>

        <section className="form-card fade-up delay-3">
          <div className="form-head">
            <div>
              <h2>登录账户</h2>
              <p>输入用户名或邮箱与密码</p>
            </div>
          </div>

          <div className="tabs">
            <button className="tab tab-active" type="button">登录</button>
            <a className="tab" href="/register">注册</a>
          </div>

          <form className="form" onSubmit={twoFactor.required ? submitTwoFactor : submitLogin}>
            <Field
              label="邮箱"
              type="email"
              placeholder="you@example.com"
              value={form.email}
              onChange={(value) => updateField("email", value)}
              disabled={loadingStatus || twoFactor.required}
            />

            <PasswordField
              label="密码"
              placeholder="输入密码"
              value={form.password}
              show={showPassword}
              onToggle={() => setShowPassword((value) => !value)}
              onChange={(value) => updateField("password", value)}
              disabled={twoFactor.required}
            />

            {twoFactor.required && (
              <Field
                label="两步验证码"
                placeholder="输入 6 位验证码"
                value={twoFactor.code}
                onChange={(value) => setTwoFactor((current) => ({ ...current, code: value.replace(/\D/g, "").slice(0, 6) }))}
              />
            )}

            {status.turnstile_check && (
              <div className="field">
                <label>安全验证</label>
                <div ref={turnstileRef}></div>
              </div>
            )}

            <div className="form-links">
              <a href="/forgot-password">忘记密码？</a>
              <a href="/register">没有账户，去注册</a>
            </div>

            <button
              className="primary-btn"
              type="submit"
              disabled={submitting || loadingStatus}
            >
              {submitting ? (
                <>
                  <Loader2 className="spin" size={20} />
                  {twoFactor.required ? "正在验证" : "正在登录"}
                </>
              ) : (
                <>
                  {twoFactor.required ? "验证并登录" : "登录账户"}
                  <ArrowRight size={20} />
                </>
              )}
            </button>
          </form>

          <p className="footnote">
            登录成功后将进入 Sub2API 控制台；启用两步验证的账户可直接在此完成验证。
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

function PasswordField({ label, placeholder, value, show, onToggle, onChange, disabled = false }) {
  return (
    <div className="field password">
      <label>{label}</label>
      <div className="input-wrap">
        <input
          className="input"
          type={show ? "text" : "password"}
          placeholder={placeholder}
          value={value}
          disabled={disabled}
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
