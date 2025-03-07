# models/deepseek/deepseek_chat.py

import json
from typing import Any, List, Optional
from tools.config.load import DEEPSEEK_API_KEY

from tools.database.thread.create import create_thread
from tools.database.thread.update import update_thread
from tools.database.thread.read import get_thread_by_id, get_all_threads
from tools.database.thread.delete import delete_thread
from tools.database.message.create import create_message
from tools.database.message.update import update_message
from tools.database.message.read import get_message_by_id, get_all_messages
from tools.database.message.delete import delete_message

# Import the LangChain-based DeepSeek model
from langchain_core.messages import BaseMessage, HumanMessage, AIMessage, SystemMessage
from langchain_deepseek import ChatDeepSeek

def _join_text_chunks(content_blocks: List[dict]) -> str:
    """
    Combine JSON content blocks (e.g. [{type: 'text', text: '...'}])
    into a single text string. Customize as needed (line breaks, spacing, etc.).
    """
    # Example: join with newlines
    text_chunks = []
    for block in content_blocks:
        # block is expected to have keys: {"type": "text", "text": "..."}
        if block.get("type") == "text":
            text_chunks.append(block.get("text", ""))
    return "\n".join(text_chunks)

def _convert_to_langchain_messages(payload_messages: List[dict]) -> List[BaseMessage]:
    """
    Convert the user-provided messages into LangChain messages for the LLM.
    Each message is a dict with:
      {
        "role": "system" | "user" | "assistant",
        "content": [ { "type":"text", "text": "..." }, ... ],
        "attachments": []
      }
    """
    converted = []
    for msg in payload_messages:
        role = msg.get("role")
        # Join the text portions into a single string for the LLM
        text_str = _join_text_chunks(msg.get("content", []))

        if role == "system":
            converted.append(SystemMessage(content=text_str))
        elif role == "assistant":
            converted.append(AIMessage(content=text_str))
        else:
            # treat everything else as user/human
            converted.append(HumanMessage(content=text_str))

    return converted

def create_deepseek_chat_completion(payload: dict, files: List[Any]) -> dict:
    """
    Main function invoked by the /api/v1/chat/completions route
    when 'purpose' == 'discord-bot' and 'model' == 'deepseek-chat'.

    1) Creates a new thread in the DB with the given 'provider' and 'model'.
    2) Stores all incoming messages in the DB (attachments empty for now).
    3) Calls the DeepSeek chat model (langchain) to get an assistant response.
    4) Stores that response in DB and returns the final JSON result.

    :param payload: The parsed JSON payload from the request.
    :param files:   List of uploaded files if multipart. (Currently unused.)
    :return: A JSON-serializable dict containing the model's response and any metadata.
    """

    # -------------------
    # 1) Extract fields
    # -------------------
    provider = payload.get("provider", "deepseek")
    model_name = payload.get("model", "deepseek-chat")
    messages = payload.get("messages", [])

    # optional: purpose = payload.get("purpose")

    # For now, each /completions call will create a new thread
    # If you need to continue an existing conversation, add logic to retrieve/update a thread
    thread_data = create_thread(
        model=model_name,
        provider=provider,
        tokens_spent=0  # you can update later if you parse usage
    )
    if not thread_data or not isinstance(thread_data, list):
        return {"error": "Could not create new thread."}

    thread_id = thread_data[0]["thread_id"]

    # ---------------------------
    # 2) Store incoming messages
    # ---------------------------
    for msg in messages:
        # Attachments remain empty for now
        _ = create_message(
            thread_id=thread_id,
            role=msg.get("role", "user"),  # fallback 'user' if missing
            content=msg.get("content", []),
            attachments=[]  # ignoring attachments for text-only
        )

    # ---------------------------------------
    # 3) Convert to LangChain messages & run
    # ---------------------------------------
    conversation_msgs = _convert_to_langchain_messages(messages)

    # Instantiate the ChatDeepSeek model
    # Adjust parameters (temperature, etc.) as desired
    llm = ChatDeepSeek(
        model="deepseek-chat",
        temperature=0,
        max_tokens=None,
        timeout=None,
        max_retries=2,
        api_key=DEEPSEEK_API_KEY
    )

    # Actually call the model
    # -> returns an AIMessage, which has .content
    ai_message = llm.invoke(conversation_msgs)

    # Optionally parse out usage and reasonings from ai_message
    # usage = ai_message.usage_metadata  # e.g. {'input_tokens': X, 'output_tokens': Y, 'total_tokens': Z}
    # reasoning = ai_message.additional_kwargs.get("reasoning_content")

    # ----------------------------------------
    # 4) Store the assistant's final response
    # ----------------------------------------
    # We'll place the entire text in a single content block
    assistant_content = [{"type": "text", "text": ai_message.content}]

    created_msg = create_message(
        thread_id=thread_id,
        role="assistant",
        content=assistant_content,
        attachments=[]
    )
    if not created_msg or not isinstance(created_msg, list):
        return {"error": "Could not create an assistant message in DB."}

    # If you want to track token usage, you could do:
    # total_used = usage["total_tokens"] if usage else 0
    # update_thread(thread_id, model=model_name, provider=provider, tokens_spent=total_used)

    # -----------------------
    # 5) Return JSON result
    # -----------------------
    # Return the newly created assistant message, or wrap it in any structure you need
    return {
        "thread_id": thread_id,
        "assistant_message": created_msg[0],
        # "usage": usage,      # optional
        # "reasoning": reasoning,
    }
