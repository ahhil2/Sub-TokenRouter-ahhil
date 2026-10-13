# 鹈鹕测智

鹈鹕测试按分组和模型定时生成 HTML/SVG 动画，作品供访客查看。分组的可用率探测见[模型目录与市场](../interfaces/model_catalog_and_marketplace.md#group_availability_probe)。

## 任务与调用

管理员在 `/admin/pelican-tests` 配置现有分组、请求模型、测试间隔和公开授权。新任务暂停，默认间隔为 360 分钟，允许 5 至 10080 分钟。开启定时任务或手动执行需要公开授权。启用后立即到期；修改间隔从修改时间重新计算。

固定用户提示词为：创建一个HTML,内容是SVG绘制一个鹈鹕自行车的2D动画，你不需要任何测试

`site.PelicanService` 校验任务，app 绑定分组模型候选、模型白名单和提供商选择。分组映射与提供商映射各执行一次，结果保存请求模型、执行模型和提供商 ID。平台内部测试不经过用户 Key 计费，费用由平台承担。

作品生成使用 `provider.TestService.GenerateText`，输出预算 16384 tokens，文本上限 1 MiB，执行超时 300 秒。协议支持输出限制时传递对应字段。OpenAI Codex OAuth 和 Grok OAuth 路径使用上游支持的参数，文本字节和时间限制仍生效。普通连通性测试的输出预算保持各适配器缺省值。

每轮生成一次。自适应国产平台选择第一个启用的文本协议，Antigravity 生成使用一次请求，OpenAI 生成关闭 task 自动恢复重试。流式协议明确报告截断时记为失败。格式检查提取 HTML 或 SVG，生成结果不经过补画或质量评分。

## 执行与留存

`pelican_tasks` 保存任务和租约，`pelican_results` 保存执行及正文。runner 随应用启动和停止，五秒领取一次到期任务，每分钟维护过期租约和结果。领取事务使用 PostgreSQL advisory lock 与行锁，全实例最多两个有效租约，同一任务最多一个执行。

领取时创建 running 记录，租约为 330 秒。完成事务按租约 token 更新结果和释放任务；过期执行不能覆盖中断状态。服务停止时取消在途请求并保存中断结果。进程丢失后，维护任务将过期执行标记 interrupted，下次执行时间为恢复时刻加任务间隔。停机期间错过的周期不会集中补跑。

结果按开始时间保留 30 天。公开和管理读取同时检查有效期，清理延迟不会延长公开期限。running 记录等待执行完成或租约回收后清理。任务处于排队或运行状态时，配置修改和删除返回冲突或未找到；删除任务同时删除其全部结果。

成功、失败和中断记录完成后按任务授权自动公开，running 记录不可公开。隐藏或删除结果后，匿名详情同样不可读取。失败记录的详细错误、提供商 ID、执行模型和原始响应供管理员查看。

## HTTP 与展示

- `GET /api/v1/pelican-showcase`：公开结果，每页 24 条，支持 page、group_id、model、date。date 使用北京时间。
- `GET /api/v1/pelican-showcase/catalog`：已公开且未过期的分组及模型选项，最多 2000 项。
- `GET /api/v1/pelican-showcase/items/:id`：公开作品正文及展示信息。
- `/api/v1/admin/pelican-tests`：GET 列表、POST 创建，`/:id` 的 PUT 修改和 DELETE 删除，`/:id/run` 的 POST 排队。
- `/api/v1/admin/pelican-tests/results`：GET 历史，支持 task_id；`/results/:id` 的 GET 详情、PATCH published 和 DELETE 删除。

公开接口使用面板访客 IP 限流和 no-store，管理员接口安装管理员认证、面板限流及审计。浏览页面不会触发模型调用。列表不携带 HTML 正文，进入可见区域后按需读取详情。

公开页 `/pelican-showcase` 使用独立 iframe srcdoc。iframe 的 sandbox 仅允许脚本，不授予同源、弹窗、下载、表单或顶层导航权限。预览文档的首个 meta 注入 CSP，网络连接、嵌套 frame、worker 和外部脚本被禁用。HTML 先在惰性 template 中解析，清理外链、表单、事件属性及自动跳转标记后再装入预览文档。原始响应按文本展示。

卡片使用 IntersectionObserver 控制挂载，离屏卸载。同时最多六个动画 iframe，放大预览计入额度。画布按 1024×768 等比缩放，界面使用现有布局、选择框、弹窗和中英文词条。

SuperAHHil 黑色首页由 Caddy 提供，导航入口位于 `deploy/superahhil/site/index.html`。更新应用镜像后需要同步该文件到站点静态目录。

srcdoc 同时继承主页面 CSP。渲染器从主页面已注入的配置脚本读取 nonce，赋给作品内联 script，使脚本通过继承的策略。作品自己的 CSP 仍限定内联脚本，外部脚本即使携带 nonce 也会被作品策略拦截。主站 CSP 的 script-src 没有增加 unsafe-inline。

## 验证与发布

单元测试覆盖格式提取、任务校验、一次执行、输出上限、协议截断、沙箱文档和 HTTP 权限。数据库测试要求 `PELICAN_TEST_DSN` 指向独立 `pelican_test` 数据库，覆盖迁移重复执行、并发领取、隐藏详情、过期清理和租约恢复。

迁移只创建独立表和索引，旧应用可继续启动。部署前备份数据库和配置，应用镜像回退时保留新增表。首次部署没有测试任务，管理员添加任务后再选择执行时间。
