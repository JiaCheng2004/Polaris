<div align="center">

# Polaris

**一个无状态、配置驱动的 AI 网关，用一套统一接口连接多个模型供应商和多种模态能力。**

[![Go](https://img.shields.io/badge/Go-1.26.2-00ADD8?style=for-the-badge&logo=go)](https://go.dev/)
[![API](https://img.shields.io/badge/API-v1-2563EB?style=for-the-badge)](./docs/API_REFERENCE.md)
[![Config](https://img.shields.io/badge/Config-v2-16A34A?style=for-the-badge)](./docs/CONFIGURATION.md)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?style=for-the-badge&logo=docker&logoColor=white)](./deployments/Dockerfile)
[![License](https://img.shields.io/badge/License-AGPL--3.0-F97316?style=for-the-badge)](./LICENSE)

[快速开始](#快速开始) · [API 能力](#api-能力) · [供应商](#供应商) · [配置](#配置) · [文档导航](#文档导航)

[English](./README.md)

</div>

---

## Polaris 是什么？

Polaris 是一个用 Go 编写的 AI 网关，位于你的应用和上游 AI 供应商之间。应用只需要调用一套稳定的 Polaris API，Polaris 负责供应商密钥、模型路由、故障切换、认证、限流、用量记录、响应缓存和运行时安全边界。

它适合希望自托管统一网关的团队：既能接入多个模型供应商和多种模态，又不把产品逻辑、提示词、RAG、用户会话或业务流程塞进网关层。

### Polaris 负责什么

| 范围 | 当前已实现能力 |
| --- | --- |
| 统一 `/v1` API | Chat、Responses、Messages、Embeddings、Images、Video、Voice、Audio Sessions、Transcription、Translation、Notes、Podcasts、Music、Models、Usage、Keys 和控制平面资源。 |
| 模型路由 | 供应商原生模型 ID、别名、选择器别名、模型家族路由、请求级路由提示和配置化 fallback。 |
| 认证 | 本地无认证模式、静态 Bearer Key、外部签名请求认证、Virtual Keys 和旧版 multi-user 兼容模式。 |
| 控制平面 | Projects、Virtual Keys、Policies、Budgets、Tools、Toolsets 和 MCP Bindings。 |
| 存储 | 本地 SQLite、生产形态 PostgreSQL、内存缓存和 Redis 缓存。 |
| 运维能力 | Prometheus 指标、结构化日志、可选 OpenTelemetry tracing、Request ID、请求体大小限制、CORS 控制、Docker、Compose 和发布验证命令。 |
| Go SDK | `pkg/client` 封装了当前已发布的 HTTP API。 |

### Polaris 不负责什么

Polaris 不是工作流编排框架、提示词框架、RAG 引擎、模型托管平台、向量数据库、聊天 UI 或应用登录系统。用户登录、Google OAuth、短信验证码、SSO、产品权限、提示词、检索和业务流程应该留在你的应用中。Polaris 应该作为它们下面的网关层。

## 项目状态

当前代码库已经包含一个覆盖多供应商和多模态的运行时，并提供本地验证门禁。真实供应商验证取决于你是否具备对应密钥、额度、账单、区域可用性和供应商套餐权限。

使用时可以按这个标准理解：

- `make release-check` 证明本仓库可以在本地完成构建、测试、契约检查、配置检查、安全检查和 Docker 镜像构建。
- `make live-smoke` 只有在环境变量和供应商访问权限都具备时，才证明真实上游供应商链路可用。
- 缺少供应商密钥不会阻塞本地开源开发；只会阻塞“这个供应商已经在你的环境中 live-smoke 验证过”的声明。

## 快速开始

### 1. 前置要求

- Go `1.26.2`
- Git
- Docker Desktop 或 Docker Engine，仅在你需要 Compose 或 Docker 验证时使用
- 至少一个供应商密钥用于真实模型请求；如果使用 Ollama 这类本地供应商则不需要云端密钥

### 2. 克隆并构建

```bash
git clone https://github.com/JiaCheng2004/Polaris.git
cd Polaris
make build
```

构建产物会写入 `./bin/polaris`。

### 3. 启动本地网关

默认配置文件是 [`config/polaris.yaml`](./config/polaris.yaml)。它监听 `127.0.0.1:8080`，使用 SQLite、内存缓存，并为本地开发设置 `runtime.auth.mode: none`。

```bash
export OPENAI_API_KEY=<your-openai-key>
make run
```

在另一个终端验证：

```bash
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/v1/models
```

调用 OpenAI 兼容的 Chat 接口：

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "default-chat",
    "messages": [
      {"role": "user", "content": "Explain Polaris in one sentence."}
    ]
  }'
```

由于本地默认配置使用 `auth.mode: none`，上面的请求不需要 Polaris API Key。不要把这个配置暴露到公网。

### 4. 使用 Docker Compose

```bash
cp .env.example .env
make stack-up STACK=local
make stack-logs STACK=local
make stack-down STACK=local
```

可用 stack：

| Stack | 命令 | 用途 |
| --- | --- | --- |
| `local` | `make stack-up STACK=local` | 单个 Polaris 服务，使用 SQLite 和内存缓存。 |
| `prod` | `make stack-up STACK=prod` | 生产形态的 Polaris、PostgreSQL 和 Redis。 |
| `dev` | `make stack-up STACK=dev` | 生产形态 stack，额外包含 Prometheus、Grafana 和 pgAdmin。 |

使用 `make stack-validate STACK=<local|prod|dev>` 可以验证 Compose 文件，同时避免把插值后的密钥打印到日志中。

### 5. 使用本地 Ollama 模型

Ollama 已作为原生 Chat 供应商实现。先启动 Ollama：

```bash
ollama serve
ollama pull llama3
```

创建一个本地配置，引入 [`config/providers/ollama.yaml`](./config/providers/ollama.yaml) 并设置路由别名：

```yaml
version: 2
imports:
  - ./providers/ollama.yaml
runtime:
  server:
    host: 127.0.0.1
    port: 8080
  auth:
    mode: none
  store:
    driver: sqlite
    dsn: ./polaris.db
  cache:
    driver: memory
routing:
  aliases:
    default-chat: ollama/llama3
```

然后使用这个配置启动 Polaris：

```bash
go run ./cmd/polaris --config ./config/local.ollama.yaml
```

## API 能力

Polaris 暴露稳定的 `/v1` 网关接口，并提供 health、metrics 和 MCP 代理路由。

| 分类 | Endpoint |
| --- | --- |
| 健康检查和指标 | `GET /health`, `GET /ready`, `GET /metrics` |
| Chat 和对话 | `POST /v1/chat/completions`, `POST /v1/responses`, `POST /v1/messages`, `POST /v1/tokens/count` |
| Embeddings 和翻译 | `POST /v1/embeddings`, `POST /v1/translations` |
| 图片 | `POST /v1/images/generations`, `POST /v1/images/edits` |
| 视频 | `POST /v1/video/generations`, `GET /v1/video/generations/:id`, `GET /v1/video/generations/:id/content`, `DELETE /v1/video/generations/:id` |
| 语音和音频 | `POST /v1/audio/speech`, `POST /v1/audio/transcriptions`, `POST /v1/audio/transcriptions/stream`, `GET /v1/audio/transcriptions/stream/:id/ws`, `POST /v1/audio/sessions`, `GET /v1/audio/sessions/:id/ws` |
| 同传、笔记、播客 | `POST /v1/audio/interpreting/sessions`, `GET /v1/audio/interpreting/sessions/:id/ws`, `POST /v1/audio/notes`, `GET /v1/audio/notes/:id`, `DELETE /v1/audio/notes/:id`, `POST /v1/audio/podcasts`, `GET /v1/audio/podcasts/:id`, `GET /v1/audio/podcasts/:id/content`, `DELETE /v1/audio/podcasts/:id` |
| 音乐 | `POST /v1/music/generations`, `POST /v1/music/edits`, `POST /v1/music/stems`, `POST /v1/music/lyrics`, `POST /v1/music/plans`, `GET /v1/music/jobs/:id`, `GET /v1/music/jobs/:id/content`, `DELETE /v1/music/jobs/:id` |
| 声音资源 | `GET /v1/voices`, `GET /v1/voices/:id`, `DELETE /v1/voices/:id`, `POST /v1/voices/:id/archive`, `POST /v1/voices/:id/unarchive`, `POST /v1/voices/clones`, `POST /v1/voices/designs`, `POST /v1/voices/:id/retrain`, `POST /v1/voices/:id/activate` |
| 模型和用量 | `GET /v1/models`, `GET /v1/usage` |
| Key 和控制平面 | `POST /v1/keys`, `GET /v1/keys`, `DELETE /v1/keys/:id`, `POST /v1/projects`, `GET /v1/projects`, `POST /v1/virtual_keys`, `GET /v1/virtual_keys`, `DELETE /v1/virtual_keys/:id`, `POST /v1/policies`, `GET /v1/policies`, `POST /v1/budgets`, `GET /v1/budgets`, `POST /v1/tools`, `GET /v1/tools`, `POST /v1/toolsets`, `GET /v1/toolsets`, `POST /v1/mcp/bindings`, `GET /v1/mcp/bindings` |
| MCP Broker | `ANY /mcp/:binding_id`, `ANY /mcp/:binding_id/*path` |

完整请求和响应说明见 [`docs/API_REFERENCE.md`](./docs/API_REFERENCE.md)。机器可读 OpenAPI 契约见 [`spec/openapi/polaris.v1.yaml`](./spec/openapi/polaris.v1.yaml)。

## 供应商

供应商适配器隔离在 [`internal/provider`](./internal/provider)，通过 [`config/providers`](./config/providers) 配置，并由各自的 `registry_<provider>.go` 文件注册。

| 供应商 | 当前 Polaris 覆盖范围 |
| --- | --- |
| OpenAI | Chat、Responses、Embeddings、Images、Voice、Video、原生 realtime audio sessions。 |
| Anthropic | Chat 和 messages-compatible 对话接口。 |
| Google Gemini | Chat、Embeddings、Images。 |
| Google Vertex | Veo 视频。 |
| Amazon Bedrock | 原生 Converse Chat 和 Titan Embeddings。 |
| ByteDance / Volcengine | Chat、Images、Video、TTS、STT、Streaming STT、Realtime Audio、同传、翻译、笔记、播客、声音目录和声音资产。 |
| Qwen / DashScope | Chat 和 Images。 |
| DeepSeek、xAI、OpenRouter、Together、Groq、Fireworks、Featherless、Moonshot、GLM、Mistral、NVIDIA | 通过原生或 OpenAI-compatible 接口提供 Chat-first 适配；NVIDIA 还支持 Embeddings。 |
| Replicate | 基于 Predictions 的异步视频。 |
| MiniMax | 音乐生成、翻唱编辑和歌词。 |
| ElevenLabs | Preview 音乐生成、流式生成、计划和 stems。 |
| Ollama | 通过原生 Ollama API 提供本地 Chat。 |

供应商密钥规则、行为差异和限制见 [`docs/PROVIDERS.md`](./docs/PROVIDERS.md)。

## 模型路由

Polaris 支持三种模型命名方式：

| 类型 | 示例 | 行为 |
| --- | --- | --- |
| 供应商模型 | `openai/gpt-4o` | 精确运行这个已配置的 provider/model。 |
| 别名 | `default-chat` | 通过 `routing.aliases` 解析。 |
| 模型家族或选择器 | `gpt-5.5`, `tooling-chat` | 根据嵌入式模型目录、供应商可用性、配置选择器和请求级路由提示确定性解析。 |

模型元数据嵌入自 [`internal/provider/catalog/models.yaml`](./internal/provider/catalog/models.yaml)。验证已配置模型和别名：

```bash
make verify-models
make verify-models-json
```

## 配置

Polaris 使用 YAML `version: 2` 配置，并支持有序 imports。

| 文件或目录 | 用途 |
| --- | --- |
| [`config/polaris.yaml`](./config/polaris.yaml) | 本地开发默认配置。 |
| [`config/polaris.example.yaml`](./config/polaris.example.yaml) | 生产形态部署参考配置。 |
| [`config/polaris.live-smoke.yaml`](./config/polaris.live-smoke.yaml) | 环境变量驱动的真实供应商 smoke 配置。 |
| [`config/providers`](./config/providers) | 供应商密钥引用、transport 默认值、模型启用列表和供应商覆盖项。 |
| [`config/routing`](./config/routing) | 别名、选择器和 fallback 规则。 |
| [`schema/polaris.config.schema.json`](./schema/polaris.config.schema.json) | 给编辑器和工具使用的 JSON Schema 契约。 |
| [`schema/cue/polaris.config.cue`](./schema/cue/polaris.config.cue) | 可选 CUE 校验契约。 |

配置优先级：

1. CLI flags
2. 环境变量
3. YAML 配置和导入的 YAML 片段
4. 内置默认值

密钥应该通过 `${OPENAI_API_KEY}` 这类环境变量引用。不要提交明文供应商密钥、网关 key、admin key、TLS 材料或本地 `.env` 文件。

## 认证模式

| 模式 | 适用场景 |
| --- | --- |
| `none` | 仅本地开发。不要公网暴露。 |
| `static` | 小型私有部署，使用配置中的固定 Bearer Key。 |
| `external` | 你的平台负责登录、OAuth、短信验证码、SSO、会话和用户；Polaris 只验证签名请求声明。 |
| `virtual_keys` | Polaris 负责 Projects、Virtual Keys、Policies、Budgets、Toolsets、MCP Bindings 和审计记录。 |
| `multi-user` | 旧版数据库 API key 行的兼容路径。 |

大多数产品集成中，如果你的应用已经有用户系统，先使用 `external`；如果 Polaris 需要作为 API Key 边界，使用 `virtual_keys`。

详细配置见 [`docs/AUTHENTICATION.md`](./docs/AUTHENTICATION.md)。

## Go SDK

公共 Go SDK 位于 [`pkg/client`](./pkg/client)。

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/JiaCheng2004/Polaris/pkg/client"
)

func main() {
	ctx := context.Background()

	sdk, err := client.New(
		"http://localhost:8080",
		client.WithAPIKey(os.Getenv("POLARIS_KEY")),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := sdk.CreateChatCompletion(ctx, &client.ChatCompletionRequest{
		Model: "default-chat",
		Messages: []client.ChatMessage{
			{Role: "user", Content: client.NewTextContent("Say hello.")},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	if text := resp.Choices[0].Message.Content.Text; text != nil {
		log.Println(*text)
	}
}
```

SDK helper 覆盖 Chat、Streaming Chat、Responses、Messages、Token Counting、Embeddings、Images、Voice、Streaming Transcription、Realtime Audio Sessions、Interpreting Sessions、Video、Music、Notes、Podcasts、Models、Usage、Keys 和控制平面资源。

## 验证

Makefile 是稳定的开发命令入口：

| 命令 | 验证内容 |
| --- | --- |
| `make build` | 构建 `./bin/polaris`。 |
| `make test` | 执行 `go test -race ./...`。 |
| `make lint` | 执行固定版本的 `golangci-lint`。 |
| `make security-check` | 执行固定版本 `gosec` 和精确 allowlist。 |
| `make config-check` | 校验配置加载、imports 和模型目录绑定。 |
| `make contract-check` | 校验注册路由、OpenAPI 覆盖和 golden fixtures。 |
| `make release-check` | 执行完整本地发布门禁，包括 Docker 构建和 Compose 校验。 |
| `make live-smoke` | 在有密钥和供应商权限时执行真实供应商 smoke 测试。 |

## 仓库结构

```text
cmd/polaris/              进程入口
internal/config/          配置加载、校验、imports 和热重载
internal/modality/        共享供应商契约
internal/provider/        供应商适配器、模型目录、注册和路由
internal/gateway/         HTTP server、handlers、routes 和 middleware
internal/store/           store 接口、SQLite、PostgreSQL、内存缓存、Redis
internal/tooling/         本地工具注册表
pkg/client/               公共 Go SDK
config/                   本地、参考、供应商、路由和 smoke 配置
schema/                   JSON Schema 和 CUE 配置契约
deployments/              Docker、Compose、Prometheus、Grafana 和 pgAdmin 资源
docs/                     人类可读文档
spec/openapi/             机器可读 HTTP 契约
tests/                    contract、integration、e2e、smoke 和 load 验证
```

## 文档导航

| 文档 | 用途 |
| --- | --- |
| [`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md) | 运行时架构和可维护性规则。 |
| [`docs/API_REFERENCE.md`](./docs/API_REFERENCE.md) | 人类可读 HTTP API 契约。 |
| [`spec/openapi/polaris.v1.yaml`](./spec/openapi/polaris.v1.yaml) | 机器可读 OpenAPI 契约。 |
| [`docs/CONFIGURATION.md`](./docs/CONFIGURATION.md) | 配置格式、imports、认证、供应商和路由细节。 |
| [`docs/AUTHENTICATION.md`](./docs/AUTHENTICATION.md) | 认证模式选择和外部签名请求集成。 |
| [`docs/PROVIDERS.md`](./docs/PROVIDERS.md) | 供应商配置、行为和限制。 |
| [`docs/ADDING_PROVIDER.md`](./docs/ADDING_PROVIDER.md) | 安全添加供应商适配器的 checklist。 |
| [`docs/INTEGRATION_RECIPES.md`](./docs/INTEGRATION_RECIPES.md) | 可复制的集成路径。 |
| [`docs/LOAD_TESTING.md`](./docs/LOAD_TESTING.md) | 本地 load validation 指南。 |
| [`docs/CONTRIBUTING.md`](./docs/CONTRIBUTING.md) | 贡献规则。 |

## 贡献

保持变更小而契约清晰：

- 将供应商代码隔离在 `internal/provider/<name>/`。
- Endpoint 行为变化时，同步更新 API 文档、OpenAPI 和 contract fixtures。
- 供应商测试使用 `httptest.NewServer`；单元测试不能调用真实供应商 API。
- 不要把密钥提交到 Git。
- 面向发布的变更先运行 `make release-check`。

## 许可证

Polaris 使用 [AGPL-3.0](./LICENSE) 许可证。

---

> **文档提示：** 本 README 最后更新于 04/26/2026。供应商 API、模型可用性、价格和平台访问规则都可能随时间变化；如果本文档长期未维护，请在生产使用前以当前代码库和供应商官方文档为准重新核对运维细节。
