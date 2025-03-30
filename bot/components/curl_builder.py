# components/curl_builder.py

import discord
import os
import json
from typing import Optional

async def build_payload_and_curl(
    messages: list[discord.Message],
    api_url: str = "http://localhost:8080/api/v1/chat/completions",
    provider: str = "openai",
    model: str = "gpt-4o",
    temp_dir: str = "./downloaded_files"
) -> tuple[dict, str]:
    """
    1) Build a JSON object that looks like:
       {
         "provider": "openai",
         "model": "gpt-4o",
         "purpose": "discord-bot",
         "messages": [
             {
               "role": "user" or "assistant",
               "content": [
                    {"type": "text", "text": "..."},
                    {"type": "image_file", "image_file": {"original_filename": "image.png", "uuid": "1234.png"}},
                    ...
               ],
               "files": [
                    {"original_filename": "filename.pdf", "uuid": "1234.pdf"},
                    {"original_filename": "image.png",   "uuid": "1234.png"},
                    ...
               ]
             },
             ...
         ]
       }

    2) Returns a tuple of:
       (payload_dict, curl_command_example)
    """

    messages_json = []
    # We collect info for every file we download, so we can build the curl -F parts
    file_infos = []

    # Ensure download directory exists
    os.makedirs(temp_dir, exist_ok=True)

    for msg in messages:
        role = "assistant" if msg.author.bot else "user"

        # Prepare "content" array
        content_blocks = []
        # Prepare "files" array
        files_array = []

        # 1) If the message has text content
        if msg.content.strip():
            content_blocks.append({
                "type": "text",
                "text": msg.content
            })

        # 2) Process each file
        for file in msg.attachments:  # Note: Discord.py still uses 'attachments'
            original_filename = file.filename

            # 1) If it's strictly "message.txt", just read it and continue
            if original_filename == "message.txt":
                try:
                    # Read bytes directly from the in-memory file
                    file_bytes = await file.read()
                    file_text = file_bytes.decode("utf-8", errors="replace")
                    content_blocks.append({
                        "type": "text",
                        "text": file_text
                    })
                except Exception as e:
                    print(f"Failed to read message.txt: {e}")
                    # 2) Skip adding to files or file_infos
                    continue

            # Figure out the file extension (if any) from the original filename
            # fallback to "" if none found
            guessed_extension = os.path.splitext(original_filename)[1]
            if not guessed_extension:
                # Optionally guess from content_type
                # e.g. mimetypes.guess_extension(file.content_type or '')
                # but if that fails, just keep it empty
                pass

            # The local "uuid" is effectively: "<file.id>.<extension>"
            # If there's no extension, we'll just use the ID
            if guessed_extension:
                local_filename = f"{file.id}{guessed_extension}"
            else:
                local_filename = str(file.id)

            # Add to files array
            files_array.append({
                "original_filename": original_filename,
                "uuid": local_filename
            })

            # Download to local temp dir
            local_path = os.path.join(temp_dir, local_filename)
            try:
                await file.save(local_path)
            except Exception as e:
                print(f"Failed to save file {original_filename}: {e}")
                continue  # skip adding to file_infos if save failed

            # Always add to our file_infos so we can do a -F 'files=@...'
            file_infos.append({
                "local_path": local_path,
                "original_filename": original_filename,
                "uuid": local_filename
            })

            # If it's an image (by extension or content_type), add an "image_file" block
            if is_image(guessed_extension, file.content_type):
                content_blocks.append({
                    "type": "image_file",
                    "image_file": {
                        "original_filename": original_filename,
                        "uuid": local_filename
                    }
                })

        messages_json.append({
            "role": role,
            "content": content_blocks,
            "files": files_array
        })

    # Build final payload
    payload_dict = {
        "provider": provider,
        "model": model,
        "purpose": "discord-bot",
        "messages": messages_json
    }

    # Build a sample curl command for demonstration
    curl_cmd = _build_curl_string(api_url, payload_dict, file_infos)

    return payload_dict, curl_cmd


def is_image(extension: str, content_type: Optional[str]) -> bool:
    """
    Return True if the extension or content_type indicates an image.
    Adjust or expand this logic as needed for your environment.
    """
    image_exts = {".png", ".jpg", ".jpeg", ".gif", ".webp"}
    if extension.lower() in image_exts:
        return True
    if content_type and content_type.startswith("image/"):
        return True
    return False


def _build_curl_string(
    api_url: str,
    payload_dict: dict,
    file_infos: list[dict]
) -> str:
    """
    Builds a sample multi-line curl command of the form:

      curl -X POST "{api_url}" \
           -F 'json={ ... }' \
           -F 'files=@/path/to/local;filename=UUID' -F 'original_filename=...' -F 'uuid=...' \
           ...
    """

    # Pretty-print the JSON
    pretty_json = json.dumps(payload_dict, indent=2)
    # Escape single quotes for safe embedding in -F 'json='
    escaped_json = pretty_json.replace("'", "'\"'\"'")

    lines = []
    lines.append(f'curl -X POST "{api_url}" \\')
    lines.append(f"     -F 'json={escaped_json}' \\")

    total_files = len(file_infos)
    for i, info in enumerate(file_infos):
        local_path = info["local_path"]
        original_filename = info["original_filename"]
        uuid = info["uuid"]

        line_parts = [
            f"     -F 'files=@{local_path};filename={uuid}'",
            f"-F 'original_filename={original_filename}'",
            f"-F 'uuid={uuid}'"
        ]
        # Join them on spaces for a single line
        line = " ".join(line_parts)

        # If not the last file, append backslash
        if i < total_files - 1:
            line += " \\"
        lines.append(line)

    return "\n".join(lines)
