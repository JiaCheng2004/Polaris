# components/message_chain.py
import discord

async def build_message_chain(message: discord.Message) -> list[discord.Message]:
    """
    Recursively/iteratively build a list of messages starting with `message`
    and following its replies all the way back.

    Returns a list of Messages in *reverse chronological order*:
    [NewestMessage, ..., OldestMessage].
    """
    chain = []
    current = message

    while True:
        chain.append(current)
        # If there's a reference to another message, follow it
        if current.reference and isinstance(current.reference.resolved, discord.Message):
            current = current.reference.resolved
        elif current.reference and current.reference.message_id:
            try:
                current = await current.channel.fetch_message(current.reference.message_id)
            except (discord.NotFound, discord.HTTPException):
                break
        else:
            break

    return chain
