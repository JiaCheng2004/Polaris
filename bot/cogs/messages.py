# cogs/message.py

import discord
from discord.ext import commands
from components.message_chain import build_message_chain
from components.curl_builder import build_payload_and_curl
import asyncio
import json

class SomeFeatureCog(commands.Cog):
    def __init__(self, bot: commands.Bot):
        self.bot = bot

    @commands.Cog.listener()
    async def on_message(self, message: discord.Message):
        if message.author == self.bot.user:
            return

        # Build the message chain
        messages = await build_message_chain(message)
        print(f"Chain IDs (newest->oldest): {[msg.id for msg in messages]}")

        # Build the payload and the curl example
        payload_dict, curl_cmd = await build_payload_and_curl(messages, model="")

        # Now execute the curl command after printing it out
        # using asyncio to avoid blocking the bot
        proc = await asyncio.create_subprocess_shell(
            curl_cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE
        )
        
        stdout, stderr = await proc.communicate()

        # Check if curl execution was successful
        if proc.returncode != 0:
            error_msg = stderr.decode().strip()
            print(f"curl command failed with return code {proc.returncode}. Error:")
            print(error_msg)
            return

        # Parse JSON response
        try:
            response_json = json.loads(stdout)
        except json.JSONDecodeError as e:
            print(f"Failed to parse JSON from curl response: {e}")
            print("Raw response was:")
            print(stdout.decode(errors='replace'))
            return

        # Check if the "code" parameter is 200
        if response_json.get("code") == 200:
            # If so, fetch the "result" parameter
            result_content = response_json.get("result", "")
            if result_content:
                await message.reply(result_content)
            else:
                await message.reply("Received 200 but 'result' was empty.")
        else:
            await message.reply(response_json.get("message", "no error message field"))

async def setup(bot: commands.Bot):
    await bot.add_cog(SomeFeatureCog(bot))
