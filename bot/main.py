import os
import json
import discord
from discord.ext import commands
import wavelink

with open("config/settings.json", "r", encoding="utf-8") as f:
    config_data = json.load(f)

PREFIX = config_data["prefix"]

intents = discord.Intents.all() if config_data["intents"].get("all", False) else discord.Intents.default()

bot = commands.Bot(command_prefix=PREFIX, intents=intents)

@bot.event
async def on_ready():
    print(f"Logged in as {bot.user} (ID: {bot.user.id})")

# Load the message_chain cog
async def load_extensions():
    await bot.load_extension("cogs.messages")

async def main():
    await load_extensions()
    await bot.start(os.getenv("BOT_TOKEN"))

if __name__ == "__main__":
    import asyncio
    asyncio.run(main())

