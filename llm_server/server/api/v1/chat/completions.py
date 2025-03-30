# server/api/v1/chat/completions.py

"""
FastAPI route to handle /api/v1/chat/completions with model routing.

Supports either:
 - multipart/form-data (with 'json' form field),
 - or raw JSON (application/json).
"""

import json
from typing import List, Optional

from fastapi import APIRouter, HTTPException, Request, UploadFile, Depends
from fastapi.responses import JSONResponse

# Model imports using standard pattern
from models.deepseek.deepseek_chat import create_deepseek_chat_completion
from models.deepseek.deepseek_reasoner import create_deepseek_reasoner_completion
# More model imports can be added as they become available

router = APIRouter()

@router.post("")
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

    # Process the request based on provider and model
    provider = payload.get("provider")
    model = payload.get("model")
    
    # Route to appropriate model handler
    if provider == "deepseek":
        if model == "deepseek-chat":
            result = create_deepseek_chat_completion(payload, files)
        elif model == "deepseek-reasoner":
            result = create_deepseek_reasoner_completion(payload, files)
        else:
            raise HTTPException(
                status_code=400, 
                detail=f"Unsupported model '{model}' for provider '{provider}'."
            )
        return JSONResponse(result)
    # Add more providers here as they become available
    # elif provider == "openai":
    #     # from models.openai.gpt import create_openai_chat_completion
    #     # result = create_openai_chat_completion(payload, files)
    #     raise HTTPException(status_code=501, detail="OpenAI not implemented.")
    # elif provider == "anthropic":
    #     # from models.anthropic.claude import create_anthropic_chat_completion
    #     # result = create_anthropic_chat_completion(payload, files)
    #     raise HTTPException(status_code=501, detail="Anthropic not implemented.")
    else:
        raise HTTPException(
            status_code=400,
            detail=f"Unsupported provider: '{provider}'."
        )
