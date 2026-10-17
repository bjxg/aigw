# 请求日志客户端识别设计

日期：2026-10-17

## 目标与边界

客户端零配置，在 /manage/monitor/request-logs 展示调用网关的客户端产品与独立版本。名称使用 client 前缀，避免与主 Agent、子 Agent 角色混淆。只使用原始入站信息；不使用上游模拟指纹、模型、提示词、API Key 或 SDK 版本推测产品。

## 数据模型

request_logs 新增 client_name、client_variant、client_version、client_user_agent 四个字符串字段，以及 client_user_agent_truncated 布尔字段。未知和历史记录使用空字符串/false。名称为规范小写产品名；variant 保留实际产品标识；版本保留原字符串。UA 正常完整保存，超过 4096 字节按 UTF-8 边界截断并标记；非法 UTF-8 替换为问号，避免数据库拒绝写入。字段不建索引，跟随日志本身的保留周期，独立于正文保存开关。

## 采集与解析

入口中间件先采集 UA 并保存不可变值快照；执行器创建 usage reporter 时复制快照，流式/非流式、成功/失败及无 token 的日志路径都通过 usage Record 传递到异步持久化层。WebSocket 使用握手请求标识，后续同连接请求沿用它。

使用集中式产品映射表和通用 product/version 提取器，不散落客户端 if/else。已支持 Codex、Claude Code、Pi、OpenCode、WorkBuddy、CodeBuddy。OpenCode 新格式通过单独的结构规则提取第三段版本，variant 保留产品标识 opencode；渠道和应用仍可从原始 UA 排查。

仅解析实际入站版本，不从服务端环境或 SDK 版本补全；unknown 视为缺失。Pi 的模型请求通常是 `pi (系统信息)`，因此只能可靠得到名称。Pi 的 originator=pi 可补充产品识别；如果 UA 是 Codex 模拟标识，不沿用它的版本。多个独立产品信号冲突时返回未知。WorkBuddy 官方桌面端复合 UA 中出现 CodeBuddy 时，以 WorkBuddy 产品及其版本为准；重复产品 token 兼容大小写、裸标识和一致版本，版本冲突仍视为未知。

通用 SDK、空 UA、无法匹配的产品均显示未知。没有自定义 Header 配置要求、没有动态规则管理页面，也不按请求正文推断。所有标识仅用于观测，不作为可信身份。后续补充规则须记录样本来源和回归测试。

## API 与页面

列表返回 client_name、client_variant、client_version。管理端查询额外启用 IncludeClientUserAgent，返回 client_user_agent 用于悬停；其他查询默认不选择、不返回原始 UA。客户端列不再可点击：名称与版本均非空时悬停显示 client_variant，否则显示 client_user_agent；提示内容不存在则显示破折号。不通过额外请求或弹窗展示。这里的“解析正确”指解析器输出完整名称与版本，前端不会重新猜测或验证客户端真实身份。

请求正文原有详情入口保留；管理端 details 查询仍从主表补充客户端字段和原始 UA。旧服务端字段缺失时前端兼容显示未知/破折号。

## 迁移、验证与回滚

复用现有 GORM 只添加缺失列的迁移机制，不删除或重建历史表，不自动回填。旧二进制可忽略新增列；回滚应用无需删列。仅在测试临时数据库验证迁移，不操作生产数据库。

验证解析边界、误识别、版本缺失、超长/UTF-8 UA、入口快照不受改写影响、日志数据库往返、正文关闭时详情可读、历史记录默认值，以及前端字段映射和页面展示。运行 Go 格式、vet、测试、构建及前端 lint、测试、构建。

## 本次开发约束

用户明确要求直接在 main 编码；两个仓库开始时均干净。git fetch 网络受限，提权同步请求被拒绝，因此基于本地 main 开发，不声称已同步远端；不推送或部署。

## 实施与验证结果

已在两个仓库的本地 main 实现入口采集、可扩展映射、usage 快照传递、主表持久化、管理详情补充、前端列表两列及详情入口。历史空元数据详情保持原响应。请求正文为空时仍可点击客户端查看 UA。新增文档已加入 .gitignore 例外，避免设计文件被忽略。

后端全量测试、vet、构建通过，gofmt 检查无输出。前端相关 18 项测试、构建、修改文件格式检查通过；lint 无错误，存在 17 条既有警告。全仓格式检查有 49 个未修改文件的既有问题，未扩大范围修复。页面测试使用 NODE_OPTIONS=--no-experimental-webstorage 解决 Node 26 与 jsdom 的环境冲突。新增测试覆盖 UA 解析、UTF-8/长度、历史表加列、列表不返回原始 UA、无正文详情、已有详情合并、入口改写隔离、reporter 快照和页面点击详情。

未提交、推送、部署，也未修改生产数据。后续识别规则覆盖待核实的客户端时，应基于真实入站样本补充测试。后续联网核实并补充的覆盖范围见下节。

## 主流客户端核实与扩展（2026-10-17）

| 客户端 | 已核实的格式 | 解析结果 |
| --- | --- | --- |
| Pi | `pi (darwin …; arm64)`、`pi (browser)` | pi，版本为空 |
| Pi | `pi/<version> (<platform>; <runtime>; <arch>)` | pi，仅使用请求本身版本 |
| Pi Cloudflare attribution | `pi-coding-agent` | pi，版本为空 |
| OpenCode 旧格式 | `opencode/<version>` | opencode，第二段版本 |
| OpenCode 新格式 | `opencode/<channel>/<version>/<app>` | opencode，第三段版本 |
| WorkBuddy | `WorkBuddy/<desktop-version> <product>/<version> CLI/<cli-version>` | workbuddy，桌面产品版本，不取 CLI 版本 |
| CodeBuddy | `CLI/unknown CodeBuddy/<version>` | codebuddy，不误归为 WorkBuddy |

### 来源与证据

1. Pi 官方仓库已从 badlogic/pi-mono 迁移到 earendil-works/pi。联网读取的固定快照：
   - [模型请求 UA 构造](https://github.com/earendil-works/pi/blob/8efd314fa3d3232ce197d8d7a6a5660530b9d83e/packages/ai/src/utils/pi-user-agent.ts)：返回 pi + OS 信息，不包含包版本。
   - [OpenAI adapter](https://github.com/earendil-works/pi/blob/8efd314fa3d3232ce197d8d7a6a5660530b9d83e/packages/ai/src/api/openai-completions.ts)、[Responses adapter](https://github.com/earendil-works/pi/blob/8efd314fa3d3232ce197d8d7a6a5660530b9d83e/packages/ai/src/api/openai-responses.ts)：默认采用上述 UA，允许调用方覆盖。
   - [Codex adapter](https://github.com/earendil-works/pi/blob/8efd314fa3d3232ce197d8d7a6a5660530b9d83e/packages/ai/src/api/openai-codex-responses.ts)：默认 originator=pi，与 Pi UA 一起发送；不能假设所有 provider 都发送该 Header。
   - [管理请求 UA](https://github.com/earendil-works/pi/blob/8efd314fa3d3232ce197d8d7a6a5660530b9d83e/packages/coding-agent/src/utils/pi-user-agent.ts)：pi/version，不能据此声称模型请求一定有版本。
   - [provider attribution](https://github.com/earendil-works/pi/blob/8efd314fa3d3232ce197d8d7a6a5660530b9d83e/packages/coding-agent/src/core/provider-attribution.ts)：Cloudflare 路径使用 pi-coding-agent。
2. OpenCode 官方仓库固定快照：
   - [App.useragent](https://github.com/anomalyco/opencode/blob/07bbd0fd166c82d87e79f053dc70e6fdb66f88ac/packages/core/src/app.ts)：明确返回 opencode/channel/version/app，默认缺失字段为 unknown。
   - [模型请求构造](https://github.com/anomalyco/opencode/blob/07bbd0fd166c82d87e79f053dc70e6fdb66f88ac/packages/core/src/session/model-request.ts)：模型请求携带 App.useragent(app)。
   - [官方更新服务文档](https://github.com/anomalyco/opencode/blob/07bbd0fd166c82d87e79f053dc70e6fdb66f88ac/services/update/README.md)：同时记录旧 opencode/version 格式。
3. WorkBuddy 官方已安装应用 5.8.3，读取应用代码，未运行或修改应用：
   - `WorkBuddy.app/Contents/Resources/app.asar` 内 `main/sidecar-manager.js`：platform=WorkBuddy，platformVersion 与 productVersion 来自桌面版本，CLI 版本只附加在扩展片段。
   - `app.asar.unpacked/cli/dist/codebuddy-lite-wb.mjs` 的 UserAgentHttpInterceptor.buildUserAgent：按 platform/version、applicationName 或 productName/version、扩展片段顺序拼接。
   - 此证据比社区 UA 猜测可靠；不将 CodeBuddy 的 IDE 通信专用 CodeBuddyCode/1.0 或浏览器预览 UA 当成模型客户端版本。
4. [CodeBuddy 社区接口样本](https://github.com/Axiaohungry/dsh-llm-codebuddy/blob/main/docs/反向代理调用WorkBuddy-API开发文档.md) 含 CLI/unknown CodeBuddy/2.136.0，产品映射单独保留 codebuddy。联网读取腾讯官方 npm 包元数据确认包 @tencent-ai/codebuddy-code 存在，但不以 npm 最新版本填入请求日志。

### 适用边界

插件、代理、自定义请求头仍可能覆盖原始客户端标识；只能根据网关实际收到的信号识别。Pi 未携带自身版本时留空是准确结果，不借用 Node、SDK 或模拟 Codex 版本。测试中的具体版本号部分是构造用例，不代表抓取了该版本真实流量；格式规则由以上来源支撑。

### 扩展后验证

后端再次执行 go test ./...、go vet ./...、go build ./... 全部通过；gofmt -l . 无输出。新增 Pi/Originator、OpenCode 新旧 UA、WorkBuddy 复合 UA、独立 CodeBuddy、版本缺失、版本冲突、多个产品冲突和入口中间件测试。前端 18 项相关测试与构建通过，客户端名称映射文件格式检查通过；lint 无错误，保留原有 17 条警告。

## 清理主表历史正文列（2026-10-17）

用户确认 request_logs.input_content、output_content 没有非空数据。已从 RequestLog 模型删除两字段；列表 has_content 仅检查 request_log_content；正文大小统计仅统计独立正文表。接口返回的 input_content/output_content 保持不变。

InitDB 在模型迁移后执行显式清理：只检查仍存在的旧列，NULL 和空字符串可删除，纯空白仍视为有数据。任一列有数据时两列都保留并记录警告。两列检查和原生 ALTER TABLE DROP COLUMN 位于同一事务；PostgreSQL 先取得表级独占锁，SQLite 利用事务避免检查后的并发写入造成误删。避免使用可能重建 SQLite 表的 GORM DropColumn。不使用 CASCADE；依赖旧列的索引等会使操作失败并整体回滚。

新数据库不再创建旧列；重复启动、只残留一列均可处理。迁移失败记录警告，服务仍可使用新正文表。回滚旧版本时，旧版自动加列机制可重新创建空列，不会将正文回填到主表。实际部署前按正常流程备份数据库，迁移可能取得表锁；本次只执行临时数据库测试，未操作生产数据库。

验证：go test ./...、go vet ./...、go build ./... 通过；gofmt 与 diff 检查通过。测试覆盖新表、双列/单列升级、重复运行、非空及空白拒绝删除、NULL 清理、依赖错误事务回滚、压缩正文读取、列表 has_content、大小统计、索引及外键级联保留。SQLite 已实测；PostgreSQL 当前未连接实例进行集成验证。

## 客户端列悬停交互验证（2026-10-17）

已移除客户端点击回调和按钮样式，复用 HoverTooltip（不要求文字溢出）且支持键盘聚焦。前端 11 项相关测试通过，覆盖完整解析显示 variant、名称/版本任一缺失显示 UA、点击不打开弹窗。后端全量测试、vet、构建通过；验证普通列表不返回 UA、管理查询返回 UA。前端构建、修改文件格式检查及 lint 通过（既有警告保留）。未部署。
