# components/curl_builder.py
import discord
import base64
import json
import os

async def build_payload_and_curl(
    messages: list[discord.Message],
    api_url: str = "http://localhost:8080/api/v1/chat/completions",
    provider: str = "openai",
    model: str = "gpt4",
    temp_dir: str = "./downloaded_attachments"
) -> tuple[dict, str]:
    """
    1) Build a JSON object that looks like:
       {
         "provider": "openai",
         "model": "gpt4",
         "messages": [
             {
               "role": "user" or "assistant",
               "content": [
                  {"type": "text", "text": "message content or file content"},
                  {"type": "image_url", "url": "data:image/...base64,..."},
                  ...
               ]
             },
             ...
         ]
       }

    2) Returns a tuple of:
       (payload_dict, curl_string_example)

       Where:
         - payload_dict can be further processed or sent via requests
         - curl_string_example is a sample command to demonstrate how to POST
           the payload_dict with file attachments to your server.
    """

    # This will hold the final array of message objects for the payload
    message_json_array = []

    # This will hold paths to all non-image attachments (EXCEPT message.txt) 
    # so you can attach them with `-F 'files=@...'` in the curl command.
    non_image_file_paths = []

    # Ensure a directory for downloads exists
    os.makedirs(temp_dir, exist_ok=True)

    # Iterate over messages in the order provided
    for msg in messages:
        # Determine the role
        role = "assistant" if msg.author.bot else "user"

        # Prepare content blocks for this message
        content_blocks = []

        # 1) If the message has text content (msg.content), add it
        if msg.content.strip():
            content_blocks.append({
                "type": "text",
                "text": msg.content
            })

        # 2) Check attachments
        for attachment in msg.attachments:
            # If the file is strictly named "message.txt", treat it as a text block
            if attachment.filename == "message.txt":
                try:
                    file_bytes = await attachment.read()
                    file_text = file_bytes.decode("utf-8", errors="replace")
                    # Add it as a new text block
                    content_blocks.append({
                        "type": "text",
                        "text": file_text
                    })
                except Exception as e:
                    print(f"Failed to read {attachment.filename}: {e}")
                # We do NOT count "message.txt" as an uploaded file
                continue

            # Otherwise, check if it’s an image
            lower_name = attachment.filename.lower()
            if any(lower_name.endswith(ext) for ext in [".png", ".jpg", ".jpeg", ".gif", ".webp"]):
                # It's an image -> read bytes, convert to base64 data URL
                try:
                    file_bytes = await attachment.read()
                    # Attempt to use attachment.content_type if available, else default
                    content_type = attachment.content_type or "image/png"
                    b64_data = base64.b64encode(file_bytes).decode("utf-8")
                    data_url = f"data:{content_type};base64,{b64_data}"

                    content_blocks.append({
                        "type": "image_url",
                        "url": "data_url"
                    })
                except Exception as e:
                    print(f"Failed to read image {attachment.filename}: {e}")
            else:
                # Non-image + not named 'message.txt' -> download it for separate file upload
                local_path = os.path.join(temp_dir, attachment.filename)
                try:
                    await attachment.save(local_path)
                    non_image_file_paths.append(local_path)
                except Exception as e:
                    print(f"Failed to save attachment {attachment.filename}: {e}")

        # Append the structured message
        message_json_array.append({
            "role": role,
            "content": content_blocks
        })

    # Build the final payload dict
    payload_dict = {
        "provider": provider,
        "model": model,
        "messages": message_json_array
    }

    # Construct a sample curl command
    curl_command = _build_curl_string(
        api_url=api_url,
        payload_dict=payload_dict,
        file_paths=non_image_file_paths
    )

    return payload_dict, curl_command


def _build_curl_string(
    api_url: str,
    payload_dict: dict,
    file_paths: list[str]
) -> str:
    """
    Builds a sample curl command:
        curl -X POST {api_url} \
             -F 'json={json_string}' \
             -F 'files=@/path/to/localfile'
             ...
    """
    # Convert the payload to a JSON string
    json_str = json.dumps(payload_dict, ensure_ascii=False)
    # Escape double quotes for safe embedding inside -F 'json="..."'
    escaped_json = json_str.replace('"', '\\"')

    parts = [
        f'curl -X POST "{api_url}" \\',
        f'     -F \'json="{escaped_json}"\''
    ]

    # Add each file with `-F 'files=@local_path'`
    for fp in file_paths:
        parts.append(f'     -F \'files=@{fp}\'')
    
    return " \\\n".join(parts)
