# server/api/v1/router.py

from fastapi import APIRouter
from server.api.v1 import status, metrics, logs, files, health
from server.api.v1.chat.router import create_chat_router

def create_v1_router() -> APIRouter:
    """
    Creates a router for version 1 of the API.
    This router unites multiple endpoints under /v1.
    """
    router = APIRouter()
    
    # Include each endpoint's router without trailing slashes
    router.include_router(status.router, prefix="/status", tags=["status"])
    router.include_router(logs.router, prefix="/logs", tags=["logs"])
    router.include_router(metrics.router, prefix="/metrics", tags=["metrics"])
    router.include_router(files.router, prefix="/files", tags=["files"])
    router.include_router(health.router, prefix="/health", tags=["health"])
    
    # Include nested routers without trailing slashes
    router.include_router(create_chat_router(), prefix="/chat")

    @router.get("/")
    def root_v1():
        """
        Root endpoint for /v1
        """
        return {"message": "Welcome to LLM Server v1"}

    return router 