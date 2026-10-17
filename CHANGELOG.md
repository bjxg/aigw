# Changelog

本文件记录 aigw-server 的显著变更。日期格式为 YYYY-MM-DD。

## 2026-10-17

### Added

- 请求日志客户端识别：`request_logs` 新增 `client_name`、`client_variant`、`client_version`、`client_user_agent`、`client_user_agent_truncated` 五个字段。入口中间件在任何上游指纹改写前采集入站 User-Agent 快照，经 usage reporter 传递到异步持久化层。
- 客户端产品解析基于集中式映射表与通用 product/version 提取器，已支持 Codex、Claude Code、Pi（含 `Originator: pi` 补充识别）、OpenCode（新旧两种 UA 格式）、WorkBuddy、CodeBuddy；多产品信号冲突时返回未知。
- 原始 UA 超过 4096 字节时按 UTF-8 字符边界截断并置 `client_user_agent_truncated = true`；非法 UTF-8 字节替换为问号，避免数据库拒绝写入；截断后解析会丢弃最后一个不完整 token，防止半截版本号误判。
- 管理端日志详情（`details`）自动合并客户端字段；普通列表返回 `client_name` / `client_variant` / `client_version`，管理端查询额外返回原始 UA。
- 新增设计文档 `docs/request-log-client-identity.md`（含 `.gitignore` 例外）。

### Changed

- `request_logs` 主表移除遗留的 `input_content`、`output_content` 列，请求正文统一存储在独立正文表 `request_log_content`；正文大小统计与 `has_content` 仅依赖正文表。
- `InitDB` 在迁移后执行安全清理：遗留列仅当存在且数据为 NULL/空字符串时在同一事务中删除（PostgreSQL 取表级独占锁）；存在实际数据（含纯空白）时保留两列并记录警告；迁移失败仅告警，不影响服务启动。
- GORM 迁移沿用只增加缺失列的策略，新库不再创建遗留列；回滚旧版本时空列可由旧版自动加列机制重建。

## 2026-10-16 及更早

历史变更见 Git 提交记录。
