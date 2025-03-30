# server/service.py

import time
from fastapi import FastAPI
from prometheus_fastapi_instrumentator import Instrumentator

from server.api.router import create_api_router

start_time = time.time()  # Record server startup time globally

def create_app() -> FastAPI:
    """
    Application factory to create and configure the FastAPI app.
    """
    app = FastAPI(
        title="LLM Server",
        description="A FastAPI application for internal LLM API endpoints",
        version="1.0.0",
        # Disable automatic redirects for trailing slashes
        openapi_url="/api/openapi.json",
        docs_url="/api/docs",
        redoc_url="/api/redoc",
        redirect_slashes=False
    )

    # Store the start_time in the app's state for use in status or anywhere
    app.state.start_time = start_time

    # Include the main API router which will handle further routing
    app.include_router(create_api_router(), prefix="/api")

    # Instrument Prometheus metrics
    Instrumentator().instrument(app).expose(app, endpoint="/metrics")

    return app

