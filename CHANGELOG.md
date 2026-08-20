# Octopus 私有化分支说明

本仓库为 [bestruirui/octopus](https://github.com/bestruirui/octopus)（AI API 网关）的私有化改造分支，用于家庭/内网环境集中管理多种 AI 渠道（OpenAI、Claude、国内中转等），提供统一 API 出口、渠道分组、模型测试、调用日志与额度统计。

## 项目整体说明

- **用途**：内网 AI 网关。上游开源项目按 Apache-2.0 许可，本分支在其基础上做私有化适配与功能增强，仅自用，不对外发布。
- **代码组织**：
  - `internal/` — Go 后端（op 业务逻辑、relay 转发、server 路由/Handler、model 数据模型、conf 配置与版本元数据）
  - `web/src/` — 前端（React + Vite，构建产物输出到 `static/out/` 并由 Go embed 打包）
  - `scripts/dockerfiles/Dockerfile.alpine` — 纯打包用 Dockerfile（编译在宿主机完成，产物复制进镜像）
- **构建与部署**（本机 /vol1/1000/docker/octopus）：
  1. 前端构建：`cd web && pnpm build`（产物到 `static/out/`）
  2. 后端编译：`go build -ldflags="-s -w -X 'github.com/bestruirui/octopus/internal/conf.BuildTime=...' -X '.../internal/conf.Commit=<sha>' -X '.../internal/conf.Version=v1.0.x'" -tags=jsoniter .`
  3. 复制二进制到 `build/docker/linux/amd64/octopus`
  4. `docker compose up -d` 部署（镜像 `octopus:local`）
- **版本管理**：本地日常改动仅 commit 到 `dev` 分支；完成一个功能的最终调整后推送到 Gitea（remote `gitea`，192.168.2.6:3000/chenlizhe/octopus），并递增 tag（`v1.0.x`）便于回滚。上游 `origin`（ghfast.top 加速）仅作参考，不推送。

## 版本历史

### v1.0.1 — 渠道日志功能完善与 500 错误修复（2026-08-10）

**问题背景**

新增"渠道日志"功能后，调用渠道日志列表接口 `GET /api/v1/log/channel-list` 返回 500：

```
{"code":500,"message":"SQL logic error: no such column: channel (1)"}
```

**根因**

Go 结构体字段 `ChannelId` 的 json tag 为 `channel`，GORM 默认按**字段名的 snake_case**（而非 json tag）映射数据库列名，因此查询生成了不存在的 `channel` 列。真实列为 `channel_id`。同理复查确认 `username`、`log_type` 等列均真实存在，无需其他改动。

**修复**

- `internal/op/log.go`：`ChannelLogList` 查询 Select 首项由 `channel` 改为 `channel_id`，并补充 `username` 等字段映射。
- 前端 `web/src/components/modules/log/index.tsx`、`web/src/api/endpoints/log.ts`：日志页新增"分组日志 / 渠道日志"两个 Tab；渠道日志展示时间、渠道、用户名、令牌名、模型、输入/输出长度、金额字段（仅调用记录），测试渠道日志一并展示；SSE 推送实现测试日志实时出现。
- 版本元数据修复：`version` 命令输出 Built Time/Commit 依赖 `internal/conf` 包变量，构建脚本曾误写 `main.*` 导致输出 unknown；已改为注入 `github.com/bestruirui/octopus/internal/conf.*`，`octopus version` 可正确显示构建时间与 commit。

**验证**

- 新镜像部署后 `docker exec octopus /app/octopus version` 输出 Built At、Commit ID 正常；
- channel-list 接口恢复 200，渠道日志表格与 SSE 实时日志线上验证通过。

### v1.0.0 — 渠道模型测试、UI 优化与私有化适配（2026-08-10）

- 新增渠道模型测试功能（分组/渠道 Tab 形态下的模型连通性测试）。
- 日志界面改造：分组日志与渠道日志双 Tab、调用记录字段展示。
- 私有化适配：登录凭据、部署方式等按内网环境调整。
- 前端迁移到 Vite 构建。
