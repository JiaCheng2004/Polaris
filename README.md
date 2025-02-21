[简体中文](README.zh-CN.md)

## Overview

This project provides a **C++ LLM (Large Language Model) server** using Drogon. It exposes various **HTTP endpoints** for interacting with different AI/LLM APIs (e.g., OpenAI, Google, Anthropic, etc.). The Docker setup includes two containers:

1. **llm_server**  
   - A C++ server running on port `8080`.  
   - Accepts both JSON and multipart/form-data requests.  
2. **bot**  
   - A placeholder container (for future expansions).  

You can run them together via `docker compose`.

---

## Prerequisites

1. **Docker & Docker Compose**  
   - Make sure you have Docker installed and running.  
   - Docker Compose should be available (`docker compose` command).

2. **API Keys**  
   - This server expects a `config.json` (originally `example.config.json`) to store your keys:
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
   - **Fill in** the actual API keys for each service you plan to use.

---

## Installation & Running

1. **Navigate to the main folder** (where `docker-compose.yml` is located):
   ```bash
   cd Polaris
   ```
   - Locate `example.config.json`.  
   - **Rename** it to `config.json`, place it under `llm_server/config/` (if not already).  
   - Put your actual API keys into `config.json`.

2. **Build and run both services**:
   ```bash
   docker compose up --build
   ```
   - This command compiles everything and starts two containers:
     - **llm_server**: The C++ server on port `8080`.  
     - **bot**: Placeholder container.

3. **Verify the server is up**:
   - Open [`http://localhost:8080/`](http://localhost:8080/) in your browser or use `curl`:
     ```bash
     curl http://localhost:8080/api/v1/status
     ```
   - You should see a JSON response indicating server status.

4. **Run only the LLM server** (optional):
   ```bash
   docker compose up --build llm_server
   ```
   - This ignores the `bot` service and runs just the LLM server.

5. **Shut down** when you’re done:
   ```bash
   docker compose down
   ```

---

## Endpoints Usage

Below are some typical endpoints your **llm_server** exposes. All are available at `http://<HOST>:8080`.

1. **POST** `/api/v1/chat/completions`  
   - **Purpose**: Query various LLM models (OpenAI, Google, etc.).  
   - **Accepts**: 
     - `application/json` in the request body, **or**  
     - `multipart/form-data` if you need to upload files.  
   - **Key JSON fields**:
     - `"model"`: Specifies which LLM model to use (e.g., `"openai-gpt-4"`, `"google-gemini-2.0-pro"`, etc.).  
     - Other fields your server might parse, like `"prompt"`, `"temperature"`, etc.  

   ### Example 1: Simple JSON POST
   ```bash
   curl -X POST http://localhost:8080/api/v1/chat/completions \
        -H "Content-Type: application/json" \
        -d '{
              "model": "openai-gpt-4",
              "prompt": "Hello from cURL!"
            }'
   ```
   ### Example 2: Multipart with file upload
   ```bash
   curl -X POST http://localhost:8080/api/v1/chat/completions \
        -F 'json={"model":"openai-gpt-4","prompt":"Analyze this file"}' \
        -F 'files=@/path/to/localfile.pdf'
   ```

   ### Example Response
   ```json
   {
     "model": "openai-gpt-4",
     "message": "Response from the AI model...",
     "files": [],
     "token_used": 42,
     "ecode": 200,
     "emessage": "",
     "model_info": {},
     "additional": {}
   }
   ```

2. **GET** `/api/v1/status`  
   - Returns a basic status and server info:
   ```bash
   curl http://localhost:8080/api/v1/status
   ```
   - Typical output:
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
   - Outputs Prometheus-like metrics in plain text:
   ```bash
   curl http://localhost:8080/metrics
   ```
   - Example output:
     ```
     # HELP llm_server_requests_total The total number of LLM requests processed.
     # TYPE llm_server_requests_total counter
     llm_server_requests_total 7
     ```

4. **GET** `/api/v1/logs?amount=50`  
   - Retrieves recent in-memory logs.  
   ```bash
   curl "http://localhost:8080/api/v1/logs?amount=10"
   ```
   - Returns a JSON array of log lines.

---

## Configuration Details

- **`config.json`**:  
  - This file holds API keys for different LLM services.  
  - By default, located under `llm_server/config/config.json`.  
  - Example:
    ```json
    {
      "openai": {
        "apikey": "<YOUR_KEY_HERE>"
      },
      "google": {
        "apikey": "<YOUR_KEY_HERE>"
      }
      // ...
    }
    ```
- **`docker-compose.yml`**:  
  - Orchestrates both `llm_server` and `bot`.  
  - Adjust ports or environment variables as needed.

---

## Models Explanation

You might see references to models like `"openai-gpt-4"`, `"google-gemini-2.0-pro"`, `"anthropic-claude"`, etc. In this server:

- **`model`** is a string field in the JSON or `multipart/form-data` that the server checks to route your request to the correct API or local LLM.  
- **API Keys**: You must populate them in `config.json` for any external service.  
- The server can handle **file uploads** (for context or data) if the model supports it.

---

## Troubleshooting

1. **Server not starting**?  
   - Check Docker logs: `docker compose logs llm_server`.  
   - Ensure port `8080` is free.  

2. **Invalid JSON** error?  
   - Confirm the JSON structure is correct.  
   - Use `-H "Content-Type: application/json"` and `-d '{...}'` in `curl`.  

3. **Multipart parse errors**?  
   - Double-check your `-F` parameters.  
   - Ensure you used `@` for file attachment: `-F 'files=@myFile.pdf'`.  

4. **API key errors**?  
   - Make sure you renamed `example.config.json` → `config.json` and inserted valid keys.

---

## Frequently Asked Questions

1. **Can the server handle large files?**  
   - Currently, it reads uploaded files into memory (`std::string`). This might be fine for moderate sizes. For huge files, consider streaming or direct-to-disk approaches.  

2. **How do I add a new model?**  
   - Typically, create a new class in `src/models/` implementing `IModel::uploadAndQuery()` or similar, then add a route check in `request_handler.cpp`.  

3. **Can I run multiple server instances?**  
   - Yes, but you’ll need distinct ports or container names.  

---

## Conclusion

You now have a working **C++ LLM server** with Docker Compose. You can:

- **Send JSON** or **upload files** to `/api/v1/chat/completions`.  
- **Check status** at `/api/v1/status`.  
- **Review logs** at `/api/v1/logs`.  
- **See metrics** at `/metrics`.  

Feel free to customize the code and integrate your own LLM providers. Happy coding!
