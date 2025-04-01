[English](README.md)

## 概述

Polaris 是一个多组件应用程序，旨在将大型语言模型（LLM）与 Discord 机器人集成，并由包括数据库、REST API、音频服务和监控在内的强大后端基础设施提供支持。Docker 配置负责编排以下服务：

1.  **`llm_server`**：核心后端服务，使用 **FastAPI (Python)** 构建，运行在 `8080` 端口。它与各种外部 LLM API（OpenAI、Google、Anthropic 等）交互，并处理文件上传/处理。
2.  **`bot`**：一个 Python Discord 机器人 (`discord.py`)，在 Discord 上与用户互动。它处理消息，管理对话历史记录，通过将文件上传到 `llm_server` 来处理附件，与 `llm_server` 通信以完成聊天，并与 `lavalink` 交互以实现音频功能。
3.  **`database`**：一个 PostgreSQL 数据库（端口 `5432`），启用了 `pgvector` 扩展，用于潜在的向量相似性搜索。存储应用程序数据。
4.  **`postgrest`**：一个运行在 `8000` 端口的服务，直接在 PostgreSQL `database` 之上提供 RESTful API。
5.  **`lavalink`**：一个独立的音频发送节点（端口 `2333`），供 Discord 机器人用于音乐播放功能。
6.  **`pgadmin`**：一个用于管理 PostgreSQL `database` 的 Web UI（端口 `5050`）。
7.  **`prometheus`**：一个监控系统（端口 `9090`），从配置的目标（如 `llm_server`）抓取指标。
8.  **`grafana`**：一个可视化平台（端口 `3000`），用于显示 Prometheus 收集的指标以及可能来自其他数据源的数据。

所有服务都通过 `docker compose` 管理。

---

## 先决条件

1.  **Docker & Docker Compose**：确保已安装、运行 Docker，并且 `docker compose` 命令可用。
2.  **`.env` 文件**：项目需要在根目录中有一个 `.env` 文件来存储 API 密钥和配置机密。创建此文件（例如，通过复制 `.env.example` 如果提供）并用必要的值填充它。关键变量包括：
    *   `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`：数据库凭证。
    *   `PGADMIN_DEFAULT_EMAIL`, `PGADMIN_DEFAULT_PASSWORD`：pgAdmin 登录信息。
    *   `GRAFANA_USER`, `GRAFANA_PASSWORD`：Grafana 登录信息。
    *   `LAVALINK_PASSWORD`：Lavalink 连接密码。
    *   `POSTGREST_JWT_SECRET`：PostgREST JWT 认证的密钥。
    *   `BOT_TOKEN`：您的 Discord 机器人令牌。
    *   `OPENAI_API_KEY`, `DEEPSEEK_API_KEY`, `GOOGLE_API_KEY`, `ANTHROPIC_API_KEY` 等：您打算通过 `llm_server` 使用的相应 LLM 服务的 API 密钥。
    *   `TAVILY_API_KEY`, `FEATHERLESS_API_KEY`, `LINKUP_API_KEY`, `FIRECRAWL_API_KEY`：`llm_server` 使用的工具的 API 密钥。

---

## 安装 & 运行

1.  **克隆仓库**：
    ```bash
    git clone <repository_url>
    cd Polaris
    ```
2.  **创建并填充 `.env`**：在 `Polaris` 根目录中创建 `.env` 文件。使用"先决条件"部分列出的必需 API 密钥和凭证填充它。有关所需变量的完整列表，请参阅 `.env` 文件本身。
3.  **构建并运行服务**：
    ```bash
    docker compose up --build -d
    ```
    *   此命令根据其 Dockerfile 为 `llm_server`、`bot`、`database` 和 `postgrest` 等服务构建镜像，并在分离模式 (`-d`) 下启动 `docker-compose.yml` 中定义的所有容器。
4.  **验证服务**：
    *   **LLM 服务器**：检查其是否响应：`curl http://localhost:8080/api/v1/health`
    *   **Discord 机器人**：检查您的 Discord 服务器，看机器人是否在线。您可以使用 `!ping` 命令测试其基本响应能力。使用 `!health` 检查机器人与 LLM 服务器的连接。
    *   **pgAdmin**：访问 `http://localhost:5050` 并使用 `.env` 文件中的凭证登录。
    *   **Grafana**：访问 `http://localhost:3000` 并使用 `.env` 文件中的凭证登录。
    *   **检查日志**：查看特定服务的日志：`docker compose logs <service_name>`（例如 `docker compose logs bot`, `docker compose logs llm_server`）。
5.  **关闭**：
    ```bash
    docker compose down
    ```
    *   停止并移除容器。添加 `-v` 以同时移除卷 (`docker compose down -v`)。

---

## 用法

### Discord 机器人

在配置的 Discord 频道中与机器人互动。

*   **聊天**：提及机器人或回复其消息以进行对话。机器人通过检索消息链来维护上下文。
*   **文件附件**：随消息上传文件（图像、文本、PDF 等）。机器人将下载它们，将它们上传到 `llm_server` 的文件端点 (`/api/v1/files`)，并将文件 ID 包含在发送给 LLM 的请求中，以供分析或提供上下文。
*   **命令**：
    *   `!ping`：检查机器人是否在线且响应。
    *   `!health`：检查后端 `llm_server` 的健康状态。
    *   *（在此处添加其他自定义机器人命令，如果有）*

### LLM 服务器接口

这些接口主要由 `bot` 服务内部使用，但也可以直接交互以进行测试或其他集成。它们可在 `http://localhost:8080` 访问。

1.  **POST** `/api/v1/chat/completions`
    *   **用途**：向各种 LLM 模型发送聊天请求（在此处进行 API 请求）。
    *   **接受**：`application/json` 或 `multipart/form-data`（如果包含先前上传的文件 ID）。
    *   **关键负载字段**：
        *   `"model"`：目标 LLM 的名称（例如 `"deepseek-reasoner"`）。
        *   `"provider"`：模型的提供者（例如 `"deepseek"`）。
        *   `"messages"`：消息对象数组（包含 `role`、`content`、`attachments`，后者是文件 ID 列表）。
        *   `"purpose"`：请求的上下文（例如 `"discord-bot"`）。
        *   `"author"`：关于发起请求的用户信息。
    *   **响应**：包含 LLM 的响应内容、令牌使用情况等。

2.  **POST** `/api/v1/files`
    *   **用途**：上传文件（文件处理相关接口）以供 LLM 处理或用作上下文。
    *   **接受**：`multipart/form-data`，包含一个或多个名为 `files` 的字段。
    *   **响应**：返回一个 JSON 对象，其中包含上传文件详细信息的列表，包括它们的 `file-id`。`bot` 在随后的聊天完成请求中使用这些 ID。

3.  **GET** `/api/v1/health`
    *   **用途**：健康检查相关接口。（好吧好吧，我懂了... 也许我稍后会把它改成 `/health`！😉)
    *   **响应**：指示健康状态的 JSON。

4.  **GET** `/api/v1/metrics`
    *   **用途**：指标相关接口。以 Prometheus 格式公开指标以供抓取。

5.  **GET** `/api/v1/status`
    *   **用途**：状态相关接口。
    *   **响应**：指示服务器状态和信息的 JSON。

6.  **GET** `/api/v1/logs`
    *   **用途**：日志相关接口。检索最近的服务器日志。

### 其他服务

*   **PostgREST (`http://localhost:8000`)**：提供对 `database` 的直接 REST API 访问。访问控制通常通过使用 `POSTGREST_JWT_SECRET` 的 JWT 令牌进行管理。有关用法，请参阅 PostgREST 文档。
*   **pgAdmin (`http://localhost:5050`)**：用于数据库管理的 Web UI。
*   **Prometheus (`http://localhost:9090`)**：访问 Prometheus UI 以查看抓取的指标和目标状态。
*   **Grafana (`http://localhost:3000`)**：创建仪表板以可视化来自 Prometheus 的指标或来自 PostgreSQL 数据库的数据。

---

## 配置细节

*   **`.env`**：所有机密和基本配置参数（API 密钥、数据库凭证、服务密码）的中央文件。**请勿将此文件提交到版本控制。**
*   **`docker-compose.yml`**：定义所有服务、它们的构建、卷、网络、依赖项和环境变量（通常从 `.env` 文件获取）。
*   **`bot/main.py`**：包含 Discord 机器人逻辑，包括允许的服务器/频道 ID（目前硬编码，但可以移至配置）。
*   **`llm_server/config/config.json`**：可能仍由 FastAPI `llm_server` 用于非机密配置，但 API 密钥应主要从 `docker-compose.yml` 中定义的环境变量（并因此从 `.env`）获取。*（如果可能，请验证 `llm_server` 的实际配置加载机制）*。
*   **`database/001_db_schema.sql`**：定义初始数据库模式，在 `database` 容器首次启动时执行。
*   **`postgrest/postgrest.conf`**：PostgREST 服务的配置。
*   **`lavalink/application.yml`**：Lavalink 服务器的配置。
*   **`prometheus.yml`**：Prometheus 的配置，定义抓取目标（如 `llm_server`）。
*   **`grafana.yml`**：为 Grafana 配置定义 Prometheus 数据源。

---

## 实用工具

*   **`clear_openai_files.sh`**：位于根目录中的 shell 脚本，用于帮助管理通过其 API 上传到 OpenAI 的文件。它列出与您的 `OPENAI_API_KEY`（从 `.env` 获取）关联的所有文件，并以交互方式提示删除。需要 `curl` 和 `jq`。使用 `bash clear_openai_files.sh` 运行。

---

## 故障排除

1.  **服务无法启动**：使用 `docker compose logs <service_name>` 检查日志。常见问题包括缺少 `.env` 变量、凭证不正确、端口冲突或构建错误。
2.  **机器人离线**：
    *   检查 `docker compose logs bot`。
    *   确保 `.env` 中的 `BOT_TOKEN` 正确且有效。
    *   验证 Discord 意图在 `bot/main.py` 和 Discord 开发者门户中配置正确。
3.  **LLM 服务器错误**：
    *   检查 `docker compose logs llm_server`。
    *   确保为要使用的模型在 `.env` 中设置了正确的 API 密钥。
    *   验证 FastAPI `llm_server` 可以访问外部 LLM API。
4.  **数据库/PostgREST 问题**：
    *   检查 `docker compose logs database` 和 `docker compose logs postgrest`。
    *   确保 `.env` 中的凭证在各服务（`database`、`postgrest`、`grafana`、`bot` 如果直接连接）之间匹配。
    *   验证 `POSTGREST_JWT_SECRET` 在用于身份验证时是否一致。
5.  **文件上传失败**：
    *   检查 `bot` 和 `llm_server` 的日志。
    *   确保 `bot/downloaded_files` 目录存在并在 `bot` 容器的上下文中具有正确的权限（Docker 管理卷，但检查路径）。
    *   验证 `llm_server` 的 `/api/v1/files` 端点是否正常工作。

---

## 总结

Polaris 提供了一个全面的平台，将 Discord 机器人与强大的 LLM 功能（由 FastAPI 后端驱动）和完整的后端堆栈集成在一起。使用 `docker compose` 管理服务并在 Discord 上与机器人互动。有关更深入的见解或故障排除，请参阅具体的配置和日志。

---