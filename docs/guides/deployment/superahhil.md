# SuperAHHil 部署与开发

本仓库保存应用源码和黑色站点页面。应用由根目录 `Dockerfile` 构建，Caddy 提供 `deploy/superahhil/site/` 中的首页、登录、注册和文档页面，其余请求反向代理到应用。

## 来源与定制

上游是 [TokenFlux/TokenRouter](https://github.com/TokenFlux/TokenRouter) `v0.2.7`，提交 `3460848ba293442910b5a292abd6f3e32161189f`。TokenRouter 基于 [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api) 开发。上游提交历史和 `LICENSE` 随源码保存。

二次开发包含兑换码购买入口、站长私信和 OpenAI Client Class，后两项分别见[私信领域文档](../../domains/site_messages.md)和[网关策略](../../domains/gateway_policy_controls.md)。私信迁移是 `backend/migrations/293_site_messages.sql`，启动时由应用执行。已执行的迁移文件和编号需要保持稳定，后续结构变更使用新的迁移文件。

## 在新环境构建

主机需要 Git、Docker Engine、Docker Compose 插件和可访问镜像及包仓库的网络。Dockerfile 固定了构建工具版本，详见[开发工具链](../../operations/development_workflow.md)。构建会下载依赖并占用缓存空间。

```bash
git clone https://github.com/ahhil2/Sub-TokenRouter-ahhil.git
cd Sub-TokenRouter-ahhil
cp deploy/superahhil/.env.example deploy/superahhil/.env
chmod 600 deploy/superahhil/.env
```

私有仓库需要先配置 GitHub 访问权限。编辑 `.env`，设置管理员邮箱、强密码，以及相互独立的数据库密码、Redis 密码、JWT 密钥和 TOTP 加密密钥。每项密钥可以分别用 `openssl rand -hex 32` 生成。样例中的空值会让 Compose 拒绝启动。

```bash
export APP_COMMIT="$(git rev-parse HEAD)"
export BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
docker compose --env-file deploy/superahhil/.env -f deploy/superahhil/compose.yaml config --quiet
docker compose --env-file deploy/superahhil/.env -f deploy/superahhil/compose.yaml build tokenrouter
docker compose --env-file deploy/superahhil/.env -f deploy/superahhil/compose.yaml up -d
curl --fail http://127.0.0.1:18080/health
```

该模板使用 `superahhil` 项目名，应用默认监听宿主机 `127.0.0.1:18080`。数据库和 Redis 通过内部网络访问。运行数据位于 `deploy/superahhil/data/`、`postgres_data/` 和 `redis_data/`。这些目录以及 `.env` 由 `.gitignore` 和 `.dockerignore` 排除。

`ADMIN_EMAIL` 和 `ADMIN_PASSWORD` 用于首次初始化。部署后的管理员密码在应用内修改，修改环境变量不会重置已有账号。JWT 和 TOTP 密钥应与数据库备份一起妥善保存，随意更换会影响登录会话或已有的双因素认证。

## 黑色页面与 HTTPS

页面源码使用站点同源 API。将页面复制到宿主机静态目录：

```bash
sudo install -d -m 755 /var/www/superahhil
sudo rsync -a --chmod=D755,F644 deploy/superahhil/site/ /var/www/superahhil/
```

编辑 `deploy/superahhil/Caddyfile.example` 的域名、静态目录和应用监听地址，再把该站点块合并进实际的 Caddy 配置。保留其他站点的配置。按主机的 Caddy 安装方式校验并重载：

```bash
sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
sudo systemctl reload caddy
```

`/`、`/login`、`/register`、`/docs`、`/styles.css` 和 `/landing-assets/*` 由 Caddy 提供。其余路径交给应用，包括 `/api/*`、`/dashboard`、`/purchase`、`/messages` 和应用生成的 `/assets/*`。把 `/assets/*` 指向静态站点会导致控制台加载失败。

站点文件包含 SuperAHHil 的公开域名、教程和联系图片。迁移到其他站点时，搜索这些文件中的 `superahhilai.top` 和联系信息后按需修改。登录和注册页面使用外部 React、Babel、Lucide 与 Cloudflare Turnstile 资源，客户端需要能访问对应资源域名。

## 后台品牌与购买地址

站点名称保存在数据库设置中。新环境在管理员设置中将站点名称及其各语言文本设为 `SuperAHHil`，对应 `site_name` 和 `site_texts.site_name`。

兑换码购买入口需要启用购买功能，并填写各语言购买地址，对应 `purchase_subscription_enabled` 和 `site_texts.purchase_subscription_url`。SuperAHHil 使用的公开地址是 `https://catfk.com/shop/EQSRAM2D`。同时检查支付功能与菜单设置，使用户能进入“充值/订阅”页面。兑换码由管理员创建，外部商店交付兑换码，用户再到站内兑换。

私信入口是 `/messages`。OpenAI 提供商在“编辑提供商 → 请求与协议”中选择 Client Class，未设置时为“不限”。Codex 分类使用入站 User-Agent 和 Originator，可伪造的请求头不能代替身份认证。

## 已有部署升级

新环境模板会创建自己的数据目录。已有生产服务应继续使用当前 Compose 项目、挂载目录、数据库凭据和安全密钥。直接启动新模板会得到一个新实例。

升级前先在源码目录构建带独立版本标签的镜像，再对生产数据库和应用数据做备份。核对[数据库迁移和恢复要求](../../operations/deployment_and_migrations.md)后，在生产 Compose 中更新应用的 `image`，重建应用服务。PostgreSQL、Redis 和 Caddy 按各自的升级计划维护。回退镜像前先判断数据库迁移是否兼容，必要时恢复配套备份。

应用健康检查通过后，检查登录、购买兑换码、私信和三种 Client Class 的行为。上游在线更新、官方预编译镜像和安装脚本会替换定制二进制，定制版应通过本仓库构建发布。

## 后续开发与上游合并

```bash
git remote add upstream https://github.com/TokenFlux/TokenRouter.git
git fetch upstream
git diff v0.2.7 -- backend frontend
make hooks
```

本地已有 `upstream` 时跳过添加。服务器整理的检出带有本地 `v0.2.7` 标签，其他检出可执行 `git fetch upstream tag v0.2.7` 获取基线。先阅读根目录 `AGENTS.md` 和[开发流程](../../operations/development_workflow.md)，再按改动范围运行测试。合并新上游前检查迁移编号、调度策略和前端设置是否冲突。CI 配置随仓库保存，推送后在 Actions 中查看实际检查结果。

数据库、用户内容、管理员凭据和服务密钥通过独立备份恢复。Git 仓库用于保存源码和配置样例。
