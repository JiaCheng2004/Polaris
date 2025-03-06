import discord

async def build_message_chain(
    message: discord.Message,
    limit: int = 20
) -> list[discord.Message]:
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

    # Reverse the list so it's [OldestMessage, ..., NewestMessage]
    return list(reversed(chain))
