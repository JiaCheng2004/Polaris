# server/api/v1/chat/router.py

from fastapi import APIRouter
from server.api.v1.chat import completions

def create_chat_router() -> APIRouter:
    """
    Creates a router for chat-related endpoints.
    """
    router = APIRouter()
    
    # Include chat endpoints
    router.include_router(completions.router, prefix="/completions", tags=["chat"])
    
    @router.get("/", tags=["chat"])
    def chat_root():
        """
        Root endpoint for /chat
        """
        return {
            "message": "Chat API endpoints",
            "endpoints": ["completions"]
        }
    
    return router 