# cogs/message.py

import discord
from discord.ext import commands
from components.message_chain import build_message_chain

class SomeFeatureCog(commands.Cog):
    def __init__(self, bot: commands.Cog):
        self.bot = bot

    @commands.Cog.listener()
    async def on_message(self, message: discord.Message):
        if message.author == self.bot.user:
            return

        chain = await build_message_chain(message)

        print(f"Chain IDs (newest->oldest): {[m.id for m in chain]}")

        await message.channel.send(
            f"Chain length: {len(chain)}\n"
        )

async def setup(bot: commands.Bot):
    await bot.add_cog(SomeFeatureCog(bot))
