# Discord LLM Bot

A Discord bot that integrates with the local LLM server to process messages and attachments, building conversation chains from replies.

## Features

- Processes Discord message chains by analyzing the reply history
- Uploads file attachments to the LLM server
- Sends structured conversation history to the LLM server for processing
- Handles responses that exceed Discord's character limit
- Configurable to work only in specific guilds and channels

## Setup

### Environment Variables

The bot requires the following environment variables:

- `BOT_TOKEN`: Your Discord bot token
- `ALLOWED_GUILD_IDS`: Comma-separated list of Discord guild IDs where the bot should respond (optional)
- `ALLOWED_CHANNEL_IDS`: Comma-separated list of Discord channel IDs where the bot should respond (optional)

### Running with Docker Compose

The bot is designed to work with the entire Polaris stack. Make sure the following services are running:

- `database`: PostgreSQL database
- `llm_server`: LLM API server
- Other required services from the docker-compose.yml

To run the entire stack:

```bash
# From the root directory of the project
docker-compose up -d
```

### Running Standalone (Development)

For development, you can run the bot standalone:

```bash
# Install dependencies
pip install -r requirements.txt

# Run the bot
python main.py
```

## Usage

The bot automatically responds to messages in allowed guilds and channels. It:

1. Builds a conversation chain from the reply history
2. Processes each message's content and attachments
3. Sends the structured conversation to the LLM server
4. Returns the LLM's response as a reply to the original message

If the LLM's response exceeds Discord's 2000 character limit, the bot will send the response as a text file attachment.

## Docker

The bot is containerized using Docker. The Docker setup includes:

- Python 3.11 slim image
- Automatic installation of dependencies
- Directory for downloaded files
- Proper command to run the bot

## License

This project is part of the Polaris platform. Refer to the main repository for license information. 