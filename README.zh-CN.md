<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="brand/logos/lockup-white.svg">
    <img src="brand/logos/lockup-black.svg" alt="SubLane" width="420">
  </picture>
</h1>

<p align="center">
  轻量级多工作空间订阅网关，优先支持 Codex。
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0--only-171717?style=flat-square&amp;labelColor=555555" alt="License: AGPL-3.0-only" height="20"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.26%2B-171717?style=flat-square&amp;logo=go&amp;logoColor=white&amp;labelColor=555555" alt="Go 1.26+" height="20"></a>
  <a href="web/package.json"><img src="https://img.shields.io/badge/React-19-171717?style=flat-square&amp;logo=react&amp;logoColor=white&amp;labelColor=555555" alt="React 19" height="20"></a>
  <a href="internal/storage/storage.go"><img src="https://img.shields.io/badge/SQLite-embedded-171717?style=flat-square&amp;logo=sqlite&amp;logoColor=white&amp;labelColor=555555" alt="SQLite embedded" height="20"></a>
  <a href="https://sublane.dev/docs/zh/roadmap"><img src="https://img.shields.io/badge/stage-internal%20testing-92400E?style=flat-square&amp;labelColor=555555" alt="Stage: internal testing" height="20"></a>
</p>

<p align="center">
  <a href="README.md">English</a> · <a href="https://demo.sublane.dev/">在线演示</a> · <a href="https://github.com/murongg/SubLane">项目仓库</a> · <a href="brand/README.zh-CN.md">品牌物料</a>
</p>

SubLane 将 AI 订阅账号整合为统一网关，并用独立工作空间隔离不同团队。采用 Go + SQLite，内置 Web 管理界面，单进程运行。

## 功能

- **Codex 订阅接入：** 支持浏览器 OAuth 授权或导入凭据 JSON，提供凭据加密、连接验证、模型发现和额度快照。Claude 与 Antigravity 账号接入暂时停用，已有记录保留。
- **客户端 API：** 提供 OpenAI Responses、Chat Completions、Responses 压缩，以及 [Claude Messages 和 Gemini 生成格式](https://sublane.dev/docs/zh/protocols)。支持 HTTP/SSE，Responses 另支持 WebSocket；Claude 和 Gemini 格式不代表对应订阅账号已开放。
- **工作空间与成员：** 隔离订阅账号、账号池、密钥、用量和审计记录；支持工作空间角色、邀请注册和成员直接授权。
- **账号池：** 支持模型白名单、按模型选择账号、会话绑定、并发控制、冷却恢复和 Codex 额度感知调度。
- **网络代理：** 为账号绑定固定出口代理，支持批量导入代理和手动检测出口。
- **个人 API 密钥：** 绑定账号池或用量分配，支持到期、暂停、撤销、仅本人复制密钥及导入 CC Switch。
- **用量控制：** 设置成员请求频率与并发上限，并可选按 Token、内部配置的美元金额或预估 Codex 额度份额分配用量。
- **运维与观测：** 提供个人和工作空间用量图表、活动热力图、请求诊断、管理审计日志、备份、校验与恢复，以及 Codex 客户端版本设置。
- **管理界面：** 内置中英文 Web 界面，支持明暗主题。

## 在线演示

打开[在线演示](https://demo.sublane.dev/)，在登录页选择进入演示。可浏览示例账号、账号池和用量；演示为只读模式，不执行真实 API 调用。

## 软件截图

以下截图来自只读演示模式，使用示例数据。点击缩略图可查看原图。

| 工作空间概览 | 订阅账号 | 团队用量 |
| --- | --- | --- |
| [<img src="docs/assets/screenshots/overview-zh-light.png" alt="工作空间概览：请求统计、用量趋势与活动热力图" width="220">](docs/assets/screenshots/overview-zh-light.png) | [<img src="docs/assets/screenshots/accounts-zh-light.png" alt="订阅账号：额度快照、验证状态与并发情况" width="220">](docs/assets/screenshots/accounts-zh-light.png) | [<img src="docs/assets/screenshots/usage-zh-light.png" alt="团队用量：请求统计、Token 总量与活动热力图" width="220">](docs/assets/screenshots/usage-zh-light.png) |

## 快速开始

安装 curl 和校验工具后，运行交互式安装脚本，选择 Docker Compose 或 Linux 预编译二进制，以及不使用代理、Caddy 或 Nginx：

```sh
curl -fsSL https://raw.githubusercontent.com/murongg/SubLane/main/scripts/install.sh | bash
```

Docker 模式需要 Docker Compose；二进制模式需要 Linux 和可用的 systemd 用户管理器。没有交互终端时，脚本保持原先的“Docker 且不配置代理”默认值。未填写域名时，生成的代理配置只监听本机 HTTP。打开安装器打印的地址，按引导创建管理员并为第一个工作空间命名。接入订阅账号并将其加入账号池，再创建个人密钥接入客户端。先按[第一次使用](https://sublane.dev/docs/zh/quickstart)跑通一次调用；更多工作空间和额度限制之后再按需设置。

脚本会在 `./sublane` 安装最新正式版，没有正式版时使用最新预发布版。Docker 模式使用数据卷；二进制模式使用私有数据目录和 systemd 用户服务。脚本不会覆盖已有目录。自定义参数、代理配置、HTTPS 和升级步骤见[部署文档](https://sublane.dev/docs/zh/deployment)。

## 本地开发

需要 Go 1.26+、Node.js 24、pnpm 9.12.2。

```sh
git clone https://github.com/murongg/SubLane.git
cd SubLane
make setup
make dev
```

打开 http://127.0.0.1:5173，运行 `make check` 执行检查、测试和构建。

## 文档

想先体验界面，可运行 `SUBLANE_DEMO=true make dev`，在登录页点击「进入演示」。演示服务使用独立临时数据，仅供浏览；二进制和 Compose 启动方式见[演示模式](https://sublane.dev/docs/zh/deployment#演示模式)。

从[分步入门教程](https://sublane.dev/docs/zh/guide/installation)开始。使用文档统一维护在[文档仓库](https://github.com/murongg/sublane-website)。

- [部署与升级](https://sublane.dev/docs/zh/deployment) · [备份恢复](https://sublane.dev/docs/zh/backup)
- [订阅账号接入](https://sublane.dev/docs/zh/providers) · [Codex 订阅与客户端](https://sublane.dev/docs/zh/codex) · [客户端协议](https://sublane.dev/docs/zh/protocols) · [网络代理](https://sublane.dev/docs/zh/proxies)
- [工作空间](https://sublane.dev/docs/zh/workspaces) · [成员与授权](https://sublane.dev/docs/zh/members) · [API 密钥](https://sublane.dev/docs/zh/api-keys) · [账号池](https://sublane.dev/docs/zh/groups)
- [模型目录](https://sublane.dev/docs/zh/models) · [用量分配](https://sublane.dev/docs/zh/allocations) · [团队用量与限额](https://sublane.dev/docs/zh/team-controls)
- [日常使用](https://sublane.dev/docs/zh/usage) · [账号池与请求诊断](https://sublane.dev/docs/zh/pool-runtime) · [管理审计](https://sublane.dev/docs/zh/audit) · [系统设置](https://sublane.dev/docs/zh/settings)
- [开发指南](https://sublane.dev/docs/zh/development) · [系统架构](https://sublane.dev/docs/zh/architecture)
- [发版说明](https://sublane.dev/docs/zh/releases) · [更新日志](CHANGELOG.md)
- [参与贡献](CONTRIBUTING.md) · [安全说明](SECURITY.md)

## 协议

使用 [AGPL-3.0-only](LICENSE)。依赖和改编组件的许可见[第三方声明](THIRD_PARTY_NOTICES.md)。
