[English](README.md)

## 概述

此项目提供了一个使用 Drogon 编写的 **C++ LLM（大型语言模型）服务器**。它通过 Docker 运行并提供多个 **HTTP 接口**，可与各种 AI/LLM API（如 OpenAI、Google、Anthropic 等）交互。Docker 配置会运行两个容器：

1. **llm_server**  
   - 监听 `8080` 端口的 C++ 服务  
   - 接收 JSON 或 multipart/form-data 请求  
2. **bot**  
   - 占位容器（可用于未来的功能扩展）  

通过 `docker compose` 即可一起运行这两个容器。

---

## 先决条件

1. **Docker & Docker Compose**  
   - 确保已安装并运行 Docker  
   - 确保可使用 `docker compose` 命令  

2. **API Keys**  
   - 本服务器需要一个名为 `config.json` 的配置文件（最初提供的是 `example.config.json`）来存储各个服务的 API Key：
     ```json
     {
         "openai": {
             "apikey": ""
         },
         "google":{
             "apikey": ""
         },
         "anthropic":{
             "apikey": ""
         },
         "xai": {
             "apikey": ""
         },
         "zhipu": {
             "apikey": ""
         },
         "llama": {
             "apikey": ""
         }
     }
     ```
   - **务必**在使用前填入实际的 API Key。

---

## 安装 & 运行

1. **进入主文件夹**（包含 `docker-compose.yml`）：
   ```bash
   cd Polaris
   ```
   - 找到 `example.config.json`。  
   - 将其重命名为 `config.json` 并放在 `llm_server/config/` 文件夹下（如果尚未操作）。  
   - 在 `config.json` 中填入有效的 API Key。

2. **构建并运行所有服务**：
   ```bash
   docker compose up --build
   ```
   - 这条命令会编译并启动两个容器：
     - **llm_server**：在 `8080` 端口运行的 C++ 服务器  
     - **bot**：占位容器

3. **确认服务器已启动**：
   - 在浏览器中打开 [`http://localhost:8080/`](http://localhost:8080/) 或者使用 `curl`：
     ```bash
     curl http://localhost:8080/api/v1/health
     ```
   - 你应当能看到一个 JSON 返回结果，表示服务器健康状态。

4. **只运行 LLM 服务器**（可选）：
   ```bash
   docker compose up --build llm_server
   ```
   - 忽略 `bot` 容器，只启动 LLM 服务。

5. **完成后停止**：
   ```bash
   docker compose down
   ```

---

## 接口用法

以下是 **llm_server** 常见的几个接口，访问地址为 `http://<HOST>:8080`。

1. **POST** `/api/v1/llm/completions`  
   - **用途**：向各种 LLM 模型（OpenAI, Google 等）发送请求并获得回复。  
   - **支持**：  
     - `application/json` 格式请求体，或  
     - `multipart/form-data`（可上传文件）。  
   - **常用 JSON 字段**：
     - `"model"`：指定要使用的 LLM 模型（如 `"openai-gpt-4"`、`"google-gemini-2.0-pro"` 等）。  
     - 可能还会包括 `"prompt"`、`"temperature"` 等请求参数。

   ### 示例 1：纯 JSON 请求
   ```bash
   curl -X POST http://localhost:8080/api/v1/llm/completions \
        -H "Content-Type: application/json" \
        -d '{
              "model": "openai-gpt-4",
              "prompt": "你好，cURL测试！"
            }'
   ```

   ### 示例 2：Multipart 文件上传
   ```bash
   curl -X POST http://localhost:8080/api/v1/llm/completions \
        -F 'json={"model":"openai-gpt-4","prompt":"请分析此文件"}' \
        -F 'files=@/path/to/localfile.pdf'
   ```

   ### 响应示例
   ```json
   {
     "model": "openai-gpt-4",
     "message": "模型回复内容…",
     "files": [],
     "token_used": 42,
     "ecode": 200,
     "emessage": "",
     "model_info": {},
     "additional": {}
   }
   ```

2. **GET** `/api/v1/health`  
   - 返回服务器健康与信息：
   ```bash
   curl http://localhost:8080/api/v1/health
   ```
   - 一般会返回：
     ```json
     {
       "health_status": "healthy",
       "uptime_seconds": 1234,
       "memory_usage": "1.23 MB",
       "build_version": "v1.0.0",
       "timestamp_utc": "2025-02-19T01:23:45Z"
     }
     ```

3. **GET** `/metrics`  
   - Prometheus 格式的监控指标：
   ```bash
   curl http://localhost:8080/metrics
   ```
   - 例如：
     ```
     # HELP llm_server_requests_total The total number of LLM requests processed.
     # TYPE llm_server_requests_total counter
     llm_server_requests_total 7
     ```

4. **GET** `/api/v1/logs?amount=50`  
   - 获取服务器的最近日志记录：
   ```bash
   curl "http://localhost:8080/api/v1/logs?amount=10"
   ```
   - 返回 JSON 数组，包含多行日志内容。

---

## 配置细节

- **`config.json`**：  
  - 用于存储各 LLM 服务的 API Key。  
  - 默认放在 `llm_server/config/config.json`。  
  - 示例：
    ```json
    {
      "openai": {
        "apikey": "API密钥"
      },
      "google": {
        "apikey": "API密钥"
      }
      // ...
    }
    ```

- **`docker-compose.yml`**：  
  - 同时管理 `llm_server` 和 `bot`。  
  - 可根据需要修改端口或环境变量。

---

## 模型说明

在请求中，你可以使用 `"model"` 字段（如 `"openai-gpt-4"`、`"google-gemini-2.0-pro"`、`"anthropic-claude"` 等）告诉服务器调用对应的 API 或本地模型：

- **API Key**：若是外部服务（OpenAI，Google，Anthropic...），请在 `config.json` 中填写。  
- **文件上传**：如果模型需要上下文文件，你可以使用 `multipart/form-data` 在 `files` 字段里上传。

---

## 常见问题

1. **服务器无法启动？**  
   - 使用 `docker compose logs llm_server` 查看日志。  
   - 确认 `8080` 端口未被占用。  

3. **Multipart 文件错误？**  
   - 确保 `-F 'files=@/path/to/file'` 的写法正确。  
   - 如果文件很大，请考虑流式处理，而不是一次性加载。  

4. **API Key 相关错误？**  
   - 确认文件名从 `example.config.json` 改成了 `config.json`，并且写入了正确的 `apikey`。

---

## 总结

现在你拥有一个在 Docker Compose 中运行的 **C++ LLM 服务器**。可以通过下列操作与之交互：

- **JSON 或文件**请求 -> `/api/v1/llm/completions`  
- **查看健康状态** -> `/api/v1/health`  
- **查看日志** -> `/api/v1/logs`  
- **监控指标** -> `/metrics`  

根据需要配置更多模型或自定义逻辑，祝开发顺利！

---