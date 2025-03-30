import psutil
from fastapi import APIRouter, Request

router = APIRouter()

@router.get("")
async def get_health(request: Request):
    """
    Returns a simple health check response showing server is up and running.
    """
    return {
        "status": "healthy",
        "service": "llm-server",
        "version": "1.0.0"
    } 