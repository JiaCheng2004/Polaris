import os
import discord
import aiohttp
import asyncio
import json
from discord.ext import commands
import logging
import tempfile
import traceback
from pathlib import Path

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger('discord_bot')

# Bot configuration
DISCORD_TOKEN = os.getenv('BOT_TOKEN')
LLM_SERVER_URL = 'http://llm_server:8080'  # Docker service URL

# List of allowed guilds and channels where the bot responds
ALLOWED_GUILD_IDS = []
ALLOWED_CHANNEL_IDS = []

# Bot setup with all intents for message access
intents = discord.Intents.default()
intents.message_content = True
bot = commands.Bot(command_prefix='!', intents=intents)

# Function to build message chain (provided in requirements)
async def build_message_chain(message: discord.Message, limit: int = 20) -> list[discord.Message]:
    """
    Recursively (or iteratively) build a list of messages starting from `message`
    and following its replies all the way back, but only up to `limit` messages.

    Returns a list of Messages in chronological order: [OldestMessage, ..., NewestMessage].
    """
    chain = []
    current = message

    while len(chain) < limit:
        chain.append(current)
        if current.reference and isinstance(current.reference.resolved, discord.Message):
            current = current.reference.resolved
        elif current.reference and current.reference.message_id:
            try:
                current = await current.channel.fetch_message(current.reference.message_id)
            except (discord.NotFound, discord.HTTPException):
                break
        else:
            break

    # [OldestMessage, ..., NewestMessage]
    return list(reversed(chain))

async def upload_file_to_llm_server(file_path, file_name):
    """Upload a file to the LLM server and return the file ID."""
    try:
        # Ensure the file exists
        if not os.path.exists(file_path):
            logger.error(f"File not found: {file_path}")
            return None
            
        async with aiohttp.ClientSession() as session:
            # Create FormData with the file
            form_data = aiohttp.FormData()
            form_data.add_field('files', 
                               open(file_path, 'rb'),
                               filename=file_name,
                               content_type='application/octet-stream')
            
            # Send POST request to the LLM server
            async with session.post(f"{LLM_SERVER_URL}/api/v1/files", data=form_data) as response:
                if response.status == 200:
                    result = await response.json()
                    logger.info(f"File uploaded successfully: {result}")
                    
                    # Check if the result has the expected structure
                    if 'result' in result and isinstance(result['result'], list) and len(result['result']) > 0:
                        return result['result'][0]['file-id']
                    else:
                        logger.error(f"Unexpected response structure: {result}")
                        return None
                else:
                    error_text = await response.text()
                    logger.error(f"Failed to upload file: HTTP {response.status} - {error_text}")
                    return None
    except Exception as e:
        logger.error(f"Error uploading file {file_path}: {e}")
        logger.error(traceback.format_exc())
        return None

async def process_attachments(attachments):
    """Process all attachments from a message and return file IDs."""
    file_ids = []
    
    # Ensure download directory exists
    download_dir = Path("bot/downloaded_files")
    download_dir.mkdir(parents=True, exist_ok=True)
    
    for attachment in attachments:
        try:
            download_path = download_dir / attachment.filename
            await attachment.save(str(download_path))
            
            logger.info(f"Downloaded attachment to {download_path}")
            
            file_id = await upload_file_to_llm_server(str(download_path), attachment.filename)
            if file_id:
                file_ids.append(file_id)
                logger.info(f"Attachment uploaded with ID: {file_id}")
            else:
                logger.warning(f"Failed to get file ID for {download_path}")
        except Exception as e:
            logger.error(f"Error processing attachment {attachment.filename}: {e}")
            logger.error(traceback.format_exc())
    
    return file_ids

async def send_chat_completion_request(messages, user_id, username):
    """Send a chat completion request to the LLM server."""
    try:
        payload = {
            "model": "deepseek-reasoner",  # Default model
            "provider": "deepseek",        # Default provider
            "messages": messages,
            "purpose": "discord-bot",
            "author": {
                "type": "discord-user",
                "user-id": str(user_id),
                "name": username
            }
        }
        
        logger.info(f"Sending request to LLM server: {json.dumps(payload, indent=2)}")
        
        async with aiohttp.ClientSession() as session:
            async with session.post(
                f"{LLM_SERVER_URL}/api/v1/chat/completions", 
                json=payload,
                timeout=aiohttp.ClientTimeout(total=300)  # 5 minute timeout
            ) as response:
                if response.status == 200:
                    return await response.json()
                else:
                    error_text = await response.text()
                    logger.error(f"Error from LLM server: HTTP {response.status} - {error_text}")
                    return {"error": f"LLM server error: {error_text}"}
    except asyncio.TimeoutError:
        logger.error("Request to LLM server timed out")
        return {"error": "Request to LLM server timed out after 5 minutes"}
    except Exception as e:
        logger.error(f"Exception while sending request to LLM server: {e}")
        logger.error(traceback.format_exc())
        return {"error": f"Failed to communicate with LLM server: {str(e)}"}

async def check_llm_server_health():
    """Check if the LLM server is healthy."""
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(f"{LLM_SERVER_URL}/api/v1/health") as response:
                if response.status == 200:
                    return True, await response.json()
                else:
                    return False, await response.text()
    except Exception as e:
        return False, str(e)

@bot.event
async def on_ready():
    """Handler for when the bot is ready."""
    logger.info(f'Bot is ready! Logged in as {bot.user}')
    logger.info(f'Allowed Guild IDs: {ALLOWED_GUILD_IDS}')
    logger.info(f'Allowed Channel IDs: {ALLOWED_CHANNEL_IDS}')
    
    # Check LLM server health on startup
    healthy, result = await check_llm_server_health()
    if healthy:
        logger.info(f"LLM server is healthy: {result}")
    else:
        logger.error(f"LLM server health check failed: {result}")

@bot.command(name="ping")
async def ping(ctx):
    """Simple command to check if the bot is responsive."""
    await ctx.send("Pong! Bot is online.")

@bot.command(name="health")
async def health(ctx):
    """Check if the LLM server is healthy."""
    async with ctx.typing():
        healthy, result = await check_llm_server_health()
        if healthy:
            await ctx.send(f"✅ LLM server is healthy: {result}")
        else:
            await ctx.send(f"❌ LLM server health check failed: {result}")

@bot.event
async def on_message(message):
    """Handler for incoming messages."""
    # Ignore messages from the bot itself
    if message.author == bot.user:
        return

    # Process commands first so they always work
    await bot.process_commands(message)
    
    # Check if the message is in an allowed guild and channel
    is_allowed_guild = not ALLOWED_GUILD_IDS or (message.guild and message.guild.id in ALLOWED_GUILD_IDS)
    is_allowed_channel = not ALLOWED_CHANNEL_IDS or message.channel.id in ALLOWED_CHANNEL_IDS
    
    if is_allowed_guild and is_allowed_channel:
        # Process the message and build the chain
        async with message.channel.typing():
            try:
                # Get the message chain
                message_chain = await build_message_chain(message)
                logger.info(f"Built message chain with {len(message_chain)} messages")
                
                # Process each message in the chain
                processed_messages = []
                
                for msg in message_chain:
                    # Determine the role based on whether the message is from the bot
                    role = "assistant" if msg.author == bot.user else "user"
                    
                    # Process attachments if present
                    attachment_ids = []
                    if msg.attachments:
                        logger.info(f"Processing {len(msg.attachments)} attachments from message by {msg.author.display_name}")
                        attachment_ids = await process_attachments(msg.attachments)
                    
                    # Add the processed message
                    processed_messages.append({
                        "role": role,
                        "content": msg.content if msg.content else "",
                        "attachments": attachment_ids
                    })
                
                # Send the request to the LLM server
                logger.info(f"Sending request for {len(processed_messages)} messages to LLM server")
                response = await send_chat_completion_request(
                    processed_messages, 
                    message.author.id,
                    message.author.display_name
                )
                
                if "error" in response:
                    await message.reply(f"Error: {response['error']}")
                    return
                
                # Extract the response content
                response_content = response.get("content", "")
                logger.info(f"Received response ({len(response_content)} chars) from LLM server")
                
                # Check if the response exceeds Discord's character limit
                if len(response_content) > 2000:
                    # Create a temporary file with the response
                    with tempfile.NamedTemporaryFile(
                        delete=False, 
                        suffix=".txt", 
                        prefix=f"message-{response.get('message_id', 'response')}-", 
                        mode="w", 
                        encoding="utf-8"
                    ) as temp_file:
                        temp_file.write(response_content)
                        temp_file_path = temp_file.name
                    
                    logger.info(f"Response too long, saving to file {temp_file_path}")
                    
                    # Send the response as a file attachment
                    await message.reply(
                        content="Sorry, the response was more than 2000 characters, so I put it in this file:",
                        file=discord.File(temp_file_path)
                    )
                    
                    # Clean up the temporary file
                    os.unlink(temp_file_path)
                else:
                    # Send the response directly
                    await message.reply(response_content)
            
            except Exception as e:
                logger.error(f"Error processing message: {e}")
                logger.error(traceback.format_exc())
                await message.reply(f"Sorry, I encountered an error: {str(e)}")

if __name__ == "__main__":
    logger.info("Starting Discord bot")
    bot.run(DISCORD_TOKEN) 