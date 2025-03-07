# server/api/v1/chat/completions.py

"""
FastAPI route to handle /api/v1/chat/completions with DeepSeek model logic.

Supports either:
 - multipart/form-data (with 'json' form field),
 - or raw JSON (application/json).
"""

import json
from typing import List, Optional

from fastapi import APIRouter, HTTPException, Request, UploadFile
from fastapi.responses import JSONResponse

# Import your DeepSeek chat logic
from models.deepseek.deepseek_chat import create_deepseek_chat_completion
from models.deepseek.deepseek_reasoner import create_deepseek_reasoner_completion

router = APIRouter()

@router.post("/completions")
async def create_chat_completion(request: Request):
    """
    POST /api/v1/chat/completions

    Can handle:
    1) multipart/form-data (with a 'json' form field containing the payload)
    2) raw JSON (content-type: application/json)
    """

    content_type = request.headers.get("content-type", "").lower()
    payload = {}
    files: List[UploadFile] = []

    # ----------------------
    # (A) MULTIPART FORM
    # ----------------------
    if "multipart/form-data" in content_type:
        form = await request.form()
        # Try to parse 'json' field
        json_data = form.get("json")
        if not json_data:
            raise HTTPException(
                status_code=400,
                detail="No 'json' field found in multipart form data."
            )
        # Parse JSON string
        try:
            payload = json.loads(json_data)
        except json.JSONDecodeError:
            raise HTTPException(
                status_code=400,
                detail="Invalid JSON in 'json' form field."
            )

        # Collect all UploadFile items
        for field_name, field_value in form.multi_items():
            if hasattr(field_value, "filename"):
                # It's a file upload
                files.append(field_value)

    # ----------------------
    # (B) RAW JSON
    # ----------------------
    else:
        try:
            payload = await request.json()
        except Exception:
            raise HTTPException(
                status_code=400,
                detail="Request body must be valid JSON if not multipart."
            )

    # Now your 'payload' holds the parsed JSON from either path.
    # 'files' is a list of any UploadFile objects if multipart was used.
    # ------------------------------------------------------------

    # Extract key fields from the payload
    purpose = payload.get("purpose")
    provider = payload.get("provider")
    model = payload.get("model")

    # Example check for 'discord-bot' usage
    if purpose == "discord-bot":
        if provider == "deepseek":
            if model == "deepseek-chat":
                result = create_deepseek_chat_completion(payload, files)
            elif model == "deepseek-reasoner":
                result = create_deepseek_reasoner_completion(payload, files)
            return JSONResponse(result)
        elif model == "openai":
            raise HTTPException(status_code=501, detail="OpenAI not implemented.")
        elif model == "gemini":
            raise HTTPException(status_code=501, detail="Gemini not implemented.")
        else:
            raise HTTPException(
                status_code=400,
                detail=f"Unsupported model '{model}' for purpose '{purpose}'."
            )
    else:
        raise HTTPException(
            status_code=400,
            detail=f"Unsupported or missing 'purpose': '{purpose}'."
        )
