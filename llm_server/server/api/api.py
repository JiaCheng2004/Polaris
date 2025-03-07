# server/api/api.py

from fastapi import APIRouter
from server.api.v1 import status, metrics, logs
from server.api.v1.chat import completions

def create_v1_router() -> APIRouter:
    """
    Creates a router for version 1 of the API.
    This router unites multiple endpoints under /v1.
    """
    router = APIRouter()
    
    # Include each endpoint's router
    # Adjust the prefix or tags as needed
    router.include_router(status.router, prefix="/status", tags=["status"])
    router.include_router(logs.router, prefix="/logs", tags=["logs"])
    router.include_router(metrics.router, prefix="/matrices", tags=["matrices"])  # user requested /v1/matrices
    router.include_router(completions.router, prefix="/chat", tags=["chat"])

    @router.get("/", tags=["root"])
    def root_v1():
        """
        Basic root endpoint for /v1
        """
        return {"message": "Welcome to LLM Server v1"}

    return router
