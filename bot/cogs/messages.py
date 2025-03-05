# cogs/message.py

import discord
from discord.ext import commands
from components.message_chain import build_message_chain
from components.curl_builder import build_payload_and_curl

class SomeFeatureCog(commands.Cog):
    def __init__(self, bot: commands.Cog):
        self.bot = bot

    @commands.Cog.listener()
    async def on_message(self, message: discord.Message):
        if message.author == self.bot.user:
            return

        messages = await build_message_chain(message)

        print(f"Chain IDs (newest->oldest): {[msg.id for msg in messages]}")
        
        payload_dict, curl_example = await build_payload_and_curl(messages)

        print("\n--- CURL EXAMPLE ---")
        print(curl_example)
        print("--- END CURL EXAMPLE ---\n")

async def setup(bot: commands.Bot):
    await bot.add_cog(SomeFeatureCog(bot))
