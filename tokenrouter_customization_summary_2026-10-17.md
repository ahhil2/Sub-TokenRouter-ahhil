# TokenRouter 二次开发汇总

本文件汇总 2026 年 10 月 16 日至 17 日这一轮二次开发，从导航和创建密钥界面优化，到供应商错误文本识别、邮箱提示、使用记录调整及最后的优先级校验修复。

本文记录截至 2026-10-17 05:00:59 UTC 的源码验收状态。后续 GitHub 推送、镜像构建和人工上线另行执行，文中的“尚未”描述该时间点的状态。

- 仓库：`/opt/tokenrouter/Sub-TokenRouter-ahhil`
- 分支：`main`
- 状态：源码修改和定向验证已完成，最终版本尚未构建镜像或上线。
- 提交状态：改动保留在工作区，尚未提交或推送 GitHub。
- 时间说明：本文的执行时间均为 UTC；使用记录中的“今天”按浏览器本地日期计算。

## 一、功能总览

| 序号 | 二开内容 | 使用入口 | 实现结果 |
| --- | --- | --- | --- |
| 1 | 隐藏 GitHub 入口 | 右上角用户菜单 | 移除管理员菜单中的 GitHub 链接和图标 |
| 2 | 调整站长私信位置 | 左侧栏“我的账号” | 管理员私信入口移入该分组，移除管理区重复入口 |
| 3 | 直接修改提供商优先级 | 提供商管理列表 | 支持点击输入、加减按钮、自动保存和失败恢复 |
| 4 | 创建密钥时按厂商筛选分组 | 创建 API Key 弹窗 | 增加 Anthropic、OpenAI、国产模型、其他四类卡片 |
| 5 | 识别 HTTP 200 中的固定错误文本 | 提供商编辑 / 故障处理 | 命中后临时停调，并切换同组其他可用提供商 |
| 6 | 增加邮箱垃圾箱提示 | 邮箱验证页 | 添加指定提示语，放大“垃圾箱”三个字 |
| 7 | 前置常用使用记录筛选 | 使用记录页 | 管理员显示四项快捷筛选，普通用户显示可查询的维度 |
| 8 | 默认查看“今天” | 管理员和普通用户使用记录页 | 初始日期范围由“过去24小时”改为“今天” |

表中的功能均已写入源码。线上容器仍运行修改前的镜像，生产页面暂未应用这些改动。

## 二、导航调整

### 1. 右上角用户菜单

移除管理员菜单中的 GitHub 链接及图标。账户操作、主题切换、退出登录和新手引导继续可用。

修改文件：[AppHeader.vue](frontend/src/components/layout/AppHeader.vue)。

### 2. 站长私信

管理员的“站长私信”入口放入左侧栏“我的账号”，管理区的重复入口删除。普通用户已有的入口保持可用，继续使用 `/messages` 路由。

修改文件：[AppSidebar.vue](frontend/src/components/layout/AppSidebar.vue)。

## 三、提供商优先级直接编辑

“提供商管理”列表中的优先级单元格支持：

- 点击数值进入输入状态，回车或失焦保存，Esc 取消。
- 使用减一、加一按钮调整数值，连续操作通过 450 毫秒防抖合并请求。
- 保存期间显示状态并限制重复操作；保存失败时恢复原值并提示错误。
- 通过现有提供商更新接口提交 `{ priority }`，保存成功后更新列表数据。
- 优先级数值越小，调度优先级越高。

本轮收尾发现，步进按钮已有上限，但手动输入仍可提交超大整数。已补齐校验，允许范围为 `1` 至 `2,147,483,647`，中英文错误提示同时更新。新增测试覆盖最大值、超大值、负数、小数、无效文本及回车、失焦保存。

主要文件：

- [ProviderPriorityCell.vue](frontend/src/components/admin/provider/ProviderPriorityCell.vue)
- [ProvidersView.vue](frontend/src/views/admin/ProvidersView.vue)
- [ProviderPriorityCell.spec.ts](frontend/src/components/admin/provider/__tests__/ProviderPriorityCell.spec.ts)

## 四、创建 API Key 时按厂商筛选分组

需求最终落实在“创建 API Key”的分组选择区域，增加四类厂商卡片：

| 分类 | 归类示例 |
| --- | --- |
| Anthropic | Anthropic / Claude 品牌分组 |
| OpenAI | OpenAI 品牌分组 |
| 国产模型 | DeepSeek、Moonshot / Kimi、智谱、阿里、百度、讯飞、腾讯、360、零一、豆包、小米、MiniMax 等 |
| 其他 | Gemini、Grok、Antigravity、Qoder、跨类别混合分组和未识别品牌 |

分类优先读取分组的 `display_brand`，为空时使用分组名称。卡片显示分组数量，没有可用分组的分类置灰。切换分类时清空已选分组，下拉框显示当前分类中的分组。

普通和复合 API Key 的创建流程都使用该筛选。编辑已有 API Key 时展示完整分组列表，防止品牌分类变化后无法看到已绑定分组。

这是前端筛选功能，没有新增分组数据库字段，也没有修改分组路由规则。

主要文件：[KeysView.vue](frontend/src/views/user/KeysView.vue)、[keyGroupProviders.ts](frontend/src/utils/keyGroupProviders.ts)。

## 五、供应商错误文本识别与故障切换

### 1. 处理的问题

供应商可能返回 HTTP `200` 和合法的流式响应，但回复正文固定为：

```text
[No upstream account is currently available. Please retry later.]
```

这种响应不能仅凭 HTTP 状态判断失败。新增规则在回复发送给客户端之前检查文本，识别供应商固定的不可用提示。

### 2. 配置入口与参数

入口是“提供商管理 / 编辑 / 故障处理 / 识别响应中的错误文本”。面板向 Anthropic HTTP 提供商展示，Bedrock 使用独立协议，不展示此面板。

| 参数 | 说明 |
| --- | --- |
| `enabled` | 默认关闭，由管理员按提供商启用 |
| `texts` | 启用时填写 1 至 10 条完整错误文本，每条 8 至 512 个字符 |
| `cooldown_minutes` | 临时停调时长，1 至 1440 分钟；新配置的界面默认值为 5 分钟 |

配置存放在 `providers.extra.semantic_error`。以下是配置示例，不代表已写入生产供应商：

```json
{
  "semantic_error": {
    "enabled": true,
    "texts": [
      "[No upstream account is currently available. Please retry later.]"
    ],
    "cooldown_minutes": 5
  }
}
```

创建、单项更新、Extra 更新和批量更新入口都包含配置校验，配置保存使用现有提供商更新和缓存失效流程，无需新增数据库迁移。

### 3. 命中后的处理

1. 将本次尝试识别为内部 `503 / upstream_semantic_error`。
2. 跳过同一提供商重试，尝试同组其他可用提供商。
3. 写入提供商临时停调截止时间和原因，并通知调度器；成功持久化后更新临时停调缓存。
4. 临时窗口到期后，提供商可重新参与候选筛选；最终是否可选仍取决于其其他状态。
5. 失败尝试在成功用量观测前退出，错误提示不会作为成功回复发送给用户。

手动 `schedulable` 开关不会因该规则被修改。即使停调持久化失败，当前请求仍会尝试切换，并记录告警。本轮没有替供应商 7 在线启用规则或恢复其手动调度开关。

### 4. 识别范围与限制

- 覆盖 Anthropic Messages 的普通执行和透传，以及使用 Messages 上游的 Chat、Responses 转换。
- 忽略大小写并归一化连续空白，整条回复与规则相同时命中。单纯引用错误句、附带解释或普通相似回答不会按包含关系命中。
- `output_tokens=0` 本身不作为失败条件。
- 工具调用、思考块及已不可能匹配候选文本的普通内容结束探测。
- 从收到 HTTP 响应开始，探测最多等待 2 秒、缓存最多 64 KiB。超出窗口后放行已读数据，后续文本不再由此规则检查。
- 已经输出给客户端的回复不能通过该规则无感重放。
- 模型正常回答如果恰好与完整规则相同，也会命中。因此应填写供应商特有的错误句，避免常见短语。
- 停调原因使用规则类型标识，完整模型回复和规则文本不写入停调原因。

主要文件：

- [SemanticErrorFields.vue](frontend/src/components/provider/form/SemanticErrorFields.vue)
- [semanticErrorPolicy.ts](frontend/src/utils/semanticErrorPolicy.ts)
- [提供商配置校验](backend/internal/provider/semantic_error.go)
- [Anthropic 响应探测](backend/internal/upstream/anthropic/semantic_error.go)
- [网关停调和切换处理](backend/internal/gateway/provider/semantic_error.go)

## 六、邮箱验证码提示

在邮箱验证页 `/email-verify` 的“我们将发送验证码到……”下方增加：

> 15秒内邮箱没收到验证码请查看垃圾箱

“垃圾箱”三个字使用更大的字号和加粗显示，并补充中英文词条。验证码发送、重发和验证流程未作修改。

修改文件：[EmailVerifyView.vue](frontend/src/views/auth/EmailVerifyView.vue)。

## 七、使用记录快捷筛选

常用筛选显示在“筛选”按钮右侧，窄屏时换行：

- 管理员：按“模型、用户、提供商、分组”的顺序排列。
- 普通用户的用量明细：显示“模型、分组”，不新增用户或管理员提供商查询权限。
- 普通用户的错误请求页签：将现有的模型筛选前置。

其余条件继续放在筛选面板中。快捷筛选与面板共用筛选数据，条件计数、重置和查询操作包含这些前置条件。

主要文件：[UsageFilters.vue](frontend/src/components/admin/usage/UsageFilters.vue)、[用户 UsageView.vue](frontend/src/views/user/UsageView.vue)。

## 八、使用记录默认日期改为“今天”

管理员和普通用户打开使用记录页时，起止日期均设为浏览器本地的当天日期。

管理员重置筛选时重新计算当天日期，覆盖跨午夜后重置的情况。管理员页面链接携带 `start_date` 或 `end_date` 时，仍按链接指定的日期查询。

主要文件：[管理员 UsageView.vue](frontend/src/views/admin/UsageView.vue)、[用户 UsageView.vue](frontend/src/views/user/UsageView.vue)。

## 九、验证结果

以下区分原会话的验证和接续收尾时的验证，不能据此理解为全量 CI 已通过。

| 验证范围 | 结果与说明 |
| --- | --- |
| 初版导航、优先级与厂商筛选 | 原会话记录中的定向测试通过 |
| 错误文本规则前端设置与校验 | 原会话定向用例通过，失败用例修正后已重跑 |
| 错误文本识别、停调、网关切换 | 后端 5 个包的定向回归于 2026-10-17 04:21 UTC 成功结束 |
| 邮箱提示和快捷筛选 | 定向测试通过；管理员筛选测试的日期夹具修正后已重跑通过 |
| 默认“今天”、日期选择和查询 | 3 个测试文件，共 43 项通过 |
| 最后补修的优先级校验及列表 | 2 个测试文件，共 16 项通过，包含新增的 9 项边界测试 |
| 前端类型检查 | 原会话最终检查通过；最后的优先级上限修复后未重新执行全量类型检查 |
| ESLint 与 UI token 检查 | 原会话检查通过；最后补修改动对应的 ESLint 和 UI token 检查也通过 |
| 后端声明顺序及格式 | 原会话的声明顺序和 Go 格式检查通过 |
| 差异及文档检查 | `git diff --check` 通过，受影响工程文档的本地链接目标和显式锚点已检查 |

后端定向回归涉及：

```text
internal/upstream/anthropic
internal/provider
internal/gateway/provider
internal/gateway/httpapi
internal/app
```

这些包使用测试名称筛选运行，覆盖错误文本识别、正常零输出、会话槽位释放和透传行为，并非五个包的全量测试。最终代码尚未完成全量 CI、完整生产构建及线上实测。

## 十、镜像和生产状态

首版界面改动曾通过 Docker Compose 构建为 `tokenrouter-ui-check-tokenrouter:latest`，随后按用户要求删除，以继续开发后续功能。当前全部二开代码尚未重新构建成镜像。

生产状态核对时间：2026-10-17 05:00:59 UTC。

| 项目 | 核对结果 |
| --- | --- |
| 容器 | `tokenrouter` |
| 运行镜像 ID 前缀 | `e77a99685c44` |
| 容器启动时间 | 2026-10-15 20:20:59 UTC |
| 健康状态 | `healthy` |
| 本轮生产操作 | 未替换镜像、未重启、未上线 |
| 工作区状态 | 修改尚未提交或推送 |

后续上线前还需：

1. 为最终源码执行构建验证，并通过项目的 Docker Compose 流程准备新镜像。
2. 在隔离环境确认页面、正常响应、错误文本命中、临时停调及同组切换。
3. 由用户确认上线窗口，再替换或重启生产容器。
4. 上线后按需启用指定提供商的错误文本规则，并单独确认手动调度开关。

### CPU 限制说明

接续验证使用 0.1 CPU 配额、串行执行和主机负载升高时自动暂停。验证结束后没有遗留验证服务。

整机 CPU 仍出现过超过 80% 的采样，最后一次验证观察到的峰值为 97.6%。自身任务限额无法保证整机占用上限，不能把本次过程描述为始终低于 80%。

## 十一、工程文档

本轮已维护并复核以下工程文档：

- [前端 UI 规范](docs/architecture/frontend_ui_conventions.md)：快捷筛选布局及“今天”的日期约定。
- [Anthropic 上游](docs/interfaces/anthropic_upstream.md)：成功 HTTP 响应中的错误文本检查。
- [提供商维护](docs/operations/provider_maintenance.md)：配置、匹配规则、临时停调、故障切换及探测窗口限制。
- [运维目录](docs/operations/index.md)：提供商错误文本规则的文档入口。

本文是本轮变更和交付状态记录，放在仓库根目录。长期技术约定由上述工程文档维护。
