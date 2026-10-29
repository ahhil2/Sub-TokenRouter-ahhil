# 在新 VPS 部署升级后的 TokenRouter

此包包含基于 TokenRouter v0.4.0 合并的 SuperAHHil 定制源码及 Docker 部署模板，用于创建全新实例。用户、余额、提供商凭据、价卡等生产数据需要在新实例中自行配置。

源码来自 `/home/ubuntu/tokenrouter-upgrade-workspace-20261022` 的当前暂存树，包含尚未提交的升级修改。包内不含 Git 历史、本地依赖、缓存、验证日志、生产数据库或生产密钥。先前通过的构建与测试证据按已确认范围沿用，本次只导出和检查文件，没有重新构建镜像；新 VPS 的实际构建、网络和运行情况尚未验收。

## 准备

新 VPS 安装 Docker Engine、Docker Compose 插件和支持 Dockerfile 语法的 BuildKit，能够下载 Docker 镜像、npm 和 Go 依赖。容器内会自行安装编译工具，宿主机无需安装 Go 或 Node。源码压缩包大小不代表部署所需空间，构建镜像、依赖缓存及运行数据还会占用数 GiB，需要另外预留。

将源码包和 SHA256SUMS 上传到同一个目录，在该目录校验并解压：

```bash
sha256sum -c SHA256SUMS
tar -xzf tokenrouter-v0.4.0-superahhil-fresh-source-20261024.tar.gz
cd tokenrouter-v0.4.0-superahhil
cp deploy/superahhil/.env.example deploy/superahhil/.env
chmod 600 deploy/superahhil/.env
```

编辑 `deploy/superahhil/.env`，设置自己的 `ADMIN_EMAIL` 和 `ADMIN_PASSWORD`，并分别填写 `POSTGRES_PASSWORD`、`REDIS_PASSWORD`、`JWT_SECRET`、`TOTP_ENCRYPTION_KEY`。四项密钥应各用一次 `openssl rand -hex 32` 独立生成。空值会让 Compose 拒绝启动。保留 `APP_VERSION=0.4.0-superahhil` 和 `IMAGE_TAG=0.4.0-custom`；`APP_COMMIT=local` 适用于这个未提交的源码快照。

## 构建和初始化

在解压后的源码根目录执行：

```bash
docker compose --env-file deploy/superahhil/.env -f deploy/superahhil/compose.yaml config --quiet
docker compose --env-file deploy/superahhil/.env -f deploy/superahhil/compose.yaml build tokenrouter
docker compose --env-file deploy/superahhil/.env -f deploy/superahhil/compose.yaml up -d
docker compose --env-file deploy/superahhil/.env -f deploy/superahhil/compose.yaml ps
curl --fail http://127.0.0.1:18080/health
```

首次启动会在这台 VPS 的独立 PostgreSQL 中初始化表和迁移，并创建配置的管理员。准备数据库需要时间；健康检查未就绪时查看 `docker compose --env-file deploy/superahhil/.env -f deploy/superahhil/compose.yaml logs --tail=100 tokenrouter`。数据保存在 `deploy/superahhil/data/`、`postgres_data/` 和 `redis_data/`。

模板仅监听 `127.0.0.1:18080`。可以在个人电脑执行 `ssh -L 18080:127.0.0.1:18080 用户名@新VPS地址`，随后打开 `http://127.0.0.1:18080` 查看初始实例。公网访问需要在新 VPS 配置域名、HTTPS 和反向代理；使用自己的域名与站点设置。

后台登录后自行添加提供商、分组、价卡及其他业务配置。包内 `model_supplements.json` 已声明独立 `gpt-image-2.5` 的图片输出能力，Compose 会只读挂载它；实际请求仍需要提供商支持和适用价卡。原服务器上的价格条目 ID 6、18、20 属于数据库数据，不随此包迁移。

## 定制页面

应用功能和控制台由 Docker 镜像提供。定制黑色首页、登录、注册及文档页面在 `deploy/superahhil/site/`，需要另按 `docs/guides/deployment/superahhil.md` 配置 Caddy。这些静态页面保留原有站点品牌、公开域名和联系内容，部署为自己的站点时按需修改。该指南中的 Git 克隆步骤不适用于此快照，请直接使用本包源码。

此源码包的制作不执行当前生产站点升级；正式上线时间仍待定。
