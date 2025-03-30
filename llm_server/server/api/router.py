# server/api/router.py

from fastapi import APIRouter
from server.api.v1.router import create_v1_router

def create_api_router() -> APIRouter:
    """
    Creates the main API router that includes version-specific routers.
    This allows for future API versioning (v2, v3, etc.).
    """
    router = APIRouter()
    
    # Include version-specific routers
    # Each version router gets its own prefix
    router.include_router(create_v1_router(), prefix="/v1")
    
    # Future versions can be added here
    # router.include_router(create_v2_router(), prefix="/v2")
    
    @router.get("/", tags=["root"])
    def api_root():
        """
        Root endpoint for /api
        """
        return {
            "message": "LLM Server API",
            "versions": ["v1"]
        }
    
    return router 