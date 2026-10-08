# 站长私信

管理员和普通用户在 `/messages` 收发站内私信。顶栏图标显示未读数，侧栏提供“站长私信”入口。管理员也可以从用户列表的“发送私信”进入指定用户的会话。

## 权限与投递

每个普通用户有一个会话。用户能查看自己的消息并回复管理员。管理员共用收件箱和已读状态，通过服务端认证上下文中的角色获得权限。收件人需要是未删除且状态为 `active` 的普通用户。

管理员可以选择最多 100 个收件人，或向全部活跃普通用户发送消息。全员发送需要在界面中再次确认，单次上限为 10000 人。用户请求携带收件人列表或全员标记时返回 403。

正文去除首尾空白后，长度为 1 到 4000 个 Unicode 码点。界面按纯文本展示，支持换行和 Unicode 表情。编辑器提供常用表情选择，也支持粘贴表情。消息通过站内页面投递。

## 存储与重试

`293_site_messages.sql` 建立三个表：

- `site_message_conversations` 按用户保存最近消息 ID，以及用户端和管理员端的已读位置。
- `site_message_dispatches` 保存发送者、客户端 UUID、请求摘要和收件人数。
- `site_messages` 保存每个收件人的正文、发送者、发送方角色和时间。

同一发送者重试相同 UUID 和内容时，返回首次投递的人数。相同 UUID 携带不同内容时返回 409。首次全员发送确定收件人列表，重试时新注册的用户不会收到该次消息。

每次发送在一个数据库事务内完成。收件人有一个不可用时，整次发送回滚。事务按用户 ID 顺序锁住会话后分配消息 ID，已读位置据此保持递增。已读请求中的消息 ID 需要属于目标会话。

删除用户时，会话和消息由外键级联删除。常规软删除的用户会话从列表隐藏。消息和发送记录没有自动过期策略，运维备份需要包含这三个表。

## HTTP 与界面

以下路径以 `/api/v1/site-messages` 为前缀，均需要登录：

| 方法与路径 | 行为 |
| --- | --- |
| `GET /summary` | 当前用户或管理员收件箱的未读总数 |
| `GET /conversations` | 按最近消息倒序查询会话，支持 `search` 和 `page` |
| `GET /contacts` | 管理员搜索活跃普通用户，支持 `search` 和 `page` |
| `GET /conversations/:user_id` | 最近 50 条消息，`before` 用于加载更早记录 |
| `POST /conversations/:user_id/read` | 用 `through_id` 推进已读位置 |
| `POST /` | 用 `body`、`client_id`、`recipient_ids` 或 `all_users` 发送消息 |

发送接口使用控制面板的重操作限流。审计记录省略私信请求正文。顶栏每 30 秒刷新未读数，会话页每 15 秒刷新消息，隐藏标签页暂停轮询和已读提交。页面切换会话时丢弃旧请求结果，退出账号时清除未读状态。

## 实现与验证

服务和权限校验在 `backend/internal/site/messages.go`，数据库操作在 `site/postgres/messages.go`，HTTP 处理在 `site/httpapi/messages.go`。前端 API、状态、编辑器、发送弹窗和会话页分别位于 `api/siteMessages.ts`、`stores/siteMessages.ts`、`components/messages/` 和 `views/user/SiteMessagesView.vue`。

集成测试 `TestMessageRepository` 要求 `SITE_MESSAGES_TEST_DSN` 指向独立的 `messages_test` 数据库。测试清空私信表并创建临时用户，覆盖跨用户访问、已读位置、并发重试、批量回滚、游标分页和全员发送的收件人快照。线上验收使用临时测试用户进行单独投递，全员发送在隔离数据库验证。

迁移增加独立表。回退到同版本的旧应用镜像可以继续使用其他功能，私信数据留在数据库中。后续升级需要带上此迁移，并按[迁移同步规则](../operations/deployment_and_migrations.md)处理上游编号冲突。
