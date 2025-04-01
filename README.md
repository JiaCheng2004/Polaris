[简体中文](README.zh-CN.md)

## Overview

Polaris is a multi-component application designed to integrate Large Language Models (LLMs) with a Discord bot, supported by a robust backend infrastructure including a database, REST API, audio services, and monitoring. The Docker setup orchestrates the following services:

1.  **`llm_server`**: The core backend service built with **FastAPI (Python)**, running on port `8080`. It interfaces with various external LLM APIs (OpenAI, Google, Anthropic, etc.) and handles file processing/uploads.
2.  **`bot`**: A Python Discord bot (`discord.py`) that interacts with users on Discord. It processes messages, manages conversation history, handles file attachments by uploading them to `llm_server`, communicates with `llm_server` for chat completions, and interacts with `lavalink` for audio features.
3.  **`database`**: A PostgreSQL database (port `5432`) with the `pgvector` extension enabled for potential vector similarity searches. Stores application data.
4.  **`postgrest`**: A service running on port `8000` that provides a RESTful API directly over the PostgreSQL `database`.
5.  **`lavalink`**: A standalone audio sending node (port `2333`) used by the Discord bot for music playback features.
6.  **`pgadmin`**: A web UI (port `5050`) for managing the PostgreSQL `database`.
7.  **`prometheus`**: A monitoring system (port `9090`) that scrapes metrics from configured targets (like `llm_server`).
8.  **`grafana`**: A visualization platform (port `3000`) for displaying metrics collected by Prometheus and potentially other data sources.

All services are managed via `docker compose`.

---

## Prerequisites

1.  **Docker & Docker Compose**: Ensure Docker is installed, running, and the `docker compose` command is available.
2.  **`.env` File**: The project requires a `.env` file in the root directory to store API keys and configuration secrets. Create this file (e.g., by copying `.env.example` if provided) and populate it with the necessary values. Key variables include:
    *   `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`: Database credentials.
    *   `PGADMIN_DEFAULT_EMAIL`, `PGADMIN_DEFAULT_PASSWORD`: pgAdmin login.
    *   `GRAFANA_USER`, `GRAFANA_PASSWORD`: Grafana login.
    *   `LAVALINK_PASSWORD`: Password for Lavalink connection.
    *   `POSTGREST_JWT_SECRET`: Secret key for PostgREST JWT authentication.
    *   `BOT_TOKEN`: Your Discord bot's token.
    *   `OPENAI_API_KEY`, `DEEPSEEK_API_KEY`, `GOOGLE_API_KEY`, `ANTHROPIC_API_KEY`, etc.: API keys for the respective LLM services you intend to use via the `llm_server`.
    *   `TAVILY_API_KEY`, `FEATHERLESS_API_KEY`, `LINKUP_API_KEY`, `FIRECRAWL_API_KEY`: API keys for tools used by the `llm_server`.

---

## Installation & Running

1.  **Clone the Repository**:
    ```bash
    git clone <repository_url>
    cd Polaris
    ```
2.  **Create and Populate `.env`**: Create a `.env` file in the `Polaris` root directory. Fill it with the required API keys and credentials as listed in the Prerequisites section. Refer to the `.env` file itself for the full list of required variables.
3.  **Build and Run Services**:
    ```bash
    docker compose up --build -d
    ```
    *   This command builds the images for services like `llm_server`, `bot`, `database`, and `postgrest` based on their Dockerfiles and starts all containers defined in `docker-compose.yml` in detached mode (`-d`).
4.  **Verify Services**:
    *   **LLM Server**: Check if it's responsive: `curl http://localhost:8080/api/v1/health`
    *   **Discord Bot**: Check your Discord server to see if the bot is online. You can test its basic responsiveness with the `!ping` command. Use `!health` to check the bot's connection to the LLM server.
    *   **pgAdmin**: Access `http://localhost:5050` and log in using the credentials from your `.env` file.
    *   **Grafana**: Access `http://localhost:3000` and log in using the credentials from your `.env` file.
    *   **Check Logs**: View logs for specific services: `docker compose logs <service_name>` (e.g., `docker compose logs bot`, `docker compose logs llm_server`).
5.  **Shut Down**:
    ```bash
    docker compose down
    ```
    *   To stop and remove containers. Add `-v` to also remove volumes (`docker compose down -v`).

---

## Usage

### Discord Bot

Interact with the bot in configured Discord channels.

*   **Chat**: Mention the bot or reply to its messages to engage in conversation. The bot maintains context by retrieving the message chain.
*   **File Attachments**: Upload files (images, text, PDFs, etc.) along with your message. The bot will download them, upload them to the `llm_server`'s file endpoint (`/api/v1/files`), and include the file IDs in the request to the LLM for analysis or context.
*   **Commands**:
    *   `!ping`: Checks if the bot is online and responsive.
    *   `!health`: Checks the health status of the backend `llm_server`.
    *   *(Add other custom bot commands here if any)*

### LLM Server Endpoints

These endpoints are primarily used internally by the `bot` service but can be interacted with directly for testing or other integrations. They are available at `http://localhost:8080`.

1.  **POST** `/api/v1/chat/completions`
    *   **Purpose**: Send chat requests (make API requests) to various LLM models.
    *   **Accepts**: `application/json` or `multipart/form-data` (if file IDs are included from previous uploads).
    *   **Key Payload Fields**:
        *   `"model"`: Name of the target LLM (e.g., `"deepseek-reasoner"`).
        *   `"provider"`: The provider of the model (e.g., `"deepseek"`).
        *   `"messages"`: An array of message objects (with `role`, `content`, `attachments` which is a list of file IDs).
        *   `"purpose"`: Context for the request (e.g., `"discord-bot"`).
        *   `"author"`: Information about the user initiating the request.
    *   **Response**: Contains the LLM's response content, token usage, etc.

2.  **POST** `/api/v1/files`
    *   **Purpose**: Upload files (File handling related endpoints) to be processed or used as context by LLMs.
    *   **Accepts**: `multipart/form-data` with one or more fields named `files`.
    *   **Response**: Returns a JSON object containing a list of uploaded file details, including their `file-id`. The `bot` uses these IDs in subsequent chat completion requests.

3.  **GET** `/api/v1/health`
    *   **Purpose**: Health check related endpoints. (Okay, okay, I get it... maybe I'll change it to `/health` later! 😉)
    *   **Response**: JSON indicating health status.

4.  **GET** `/api/v1/metrics`
    *   **Purpose**: Metrics related endpoints. Exposes metrics in Prometheus format for scraping.

5.  **GET** `/api/v1/status`
    *   **Purpose**: Status related endpoints.
    *   **Response**: JSON indicating server status and info.

6.  **GET** `/api/v1/logs`
    *   **Purpose**: Log related endpoints. Retrieves recent server logs.

### Other Services

*   **PostgREST (`http://localhost:8000`)**: Provides direct REST API access to the `database`. Access control is typically managed via JWT tokens using the `POSTGREST_JWT_SECRET`. Refer to PostgREST documentation for usage.
*   **pgAdmin (`http://localhost:5050`)**: Web UI for database administration.
*   **Prometheus (`http://localhost:9090`)**: Access the Prometheus UI to view scraped metrics and target status.
*   **Grafana (`http://localhost:3000`)**: Create dashboards to visualize metrics from Prometheus or data from the PostgreSQL database.

---

## Configuration Details

*   **`.env`**: Central file for all secrets and essential configuration parameters (API keys, database credentials, service passwords). **Do not commit this file to version control.**
*   **`docker-compose.yml`**: Defines all the services, their builds, volumes, networks, dependencies, and environment variables (often sourcing from the `.env` file).
*   **`bot/main.py`**: Contains the Discord bot logic, including allowed guild/channel IDs (currently hardcoded but could be moved to config).
*   **`llm_server/config/config.json`**: May still be used by the FastAPI `llm_server` for non-secret configuration, but API keys should primarily be sourced from environment variables defined in `docker-compose.yml` (and thus from `.env`). *(Verify `llm_server`'s actual config loading mechanism if possible)*.
*   **`database/001_db_schema.sql`**: Defines the initial database schema, executed when the `database` container starts for the first time.
*   **`postgrest/postgrest.conf`**: Configuration for the PostgREST service.
*   **`lavalink/application.yml`**: Configuration for the Lavalink server.
*   **`prometheus.yml`**: Configuration for Prometheus, defining scrape targets (like `llm_server`).
*   **`grafana.yml`**: Defines the Prometheus datasource for Grafana provisioning.

---

## Utilities

*   **`clear_openai_files.sh`**: A shell script located in the root directory to help manage files uploaded to OpenAI via their API. It lists all files associated with your `OPENAI_API_KEY` (sourced from `.env`) and interactively prompts for deletion. Requires `curl` and `jq`. Run with `bash clear_openai_files.sh`.

---

## Troubleshooting

1.  **Service Not Starting**: Check logs using `docker compose logs <service_name>`. Common issues include missing `.env` variables, incorrect credentials, port conflicts, or build errors.
2.  **Bot Offline**:
    *   Check `docker compose logs bot`.
    *   Ensure `BOT_TOKEN` in `.env` is correct and valid.
    *   Verify Discord intents are correctly configured in `bot/main.py` and the Discord Developer Portal.
3.  **LLM Server Errors**:
    *   Check `docker compose logs llm_server`.
    *   Ensure the correct API keys are set in `.env` for the models you are trying to use.
    *   Verify the FastAPI `llm_server` can reach the external LLM APIs.
4.  **Database/PostgREST Issues**:
    *   Check `docker compose logs database` and `docker compose logs postgrest`.
    *   Ensure credentials in `.env` match across services (`database`, `postgrest`, `grafana`, `bot` if it connects directly).
    *   Verify `POSTGREST_JWT_SECRET` is consistent if used for auth.
5.  **File Upload Failures**:
    *   Check logs for both `bot` and `llm_server`.
    *   Ensure the `bot/downloaded_files` directory exists and has correct permissions within the `bot` container's context (Docker manages volumes, but check paths).
    *   Verify the `llm_server`'s `/api/v1/files` endpoint is working correctly.

---

## Conclusion

Polaris provides a comprehensive platform integrating a Discord bot with powerful LLM capabilities (powered by a FastAPI backend) and a full backend stack. Use `docker compose` to manage the services and interact with the bot on Discord. Refer to the specific configurations and logs for deeper insights or troubleshooting.
