# server/service.py

import time
from fastapi import FastAPI
from prometheus_fastapi_instrumentator import Instrumentator

from server.api.api import create_v1_router

start_time = time.time()  # Record server startup time globally

def create_app() -> FastAPI:
    """
    Application factory to create and configure the FastAPI app.
    """
    app = FastAPI(
        title="LLM Server",
        description="A FastAPI application for internal LLM API endpoints",
        version="1.0.0",
    )

    # Store the start_time in the app's state for use in status or anywhere
    app.state.start_time = start_time

    # Include the v1 router
    app.include_router(create_v1_router(), prefix="/v1")

    # Instrument Prometheus metrics
    Instrumentator().instrument(app).expose(app, endpoint="/metrics")

    return app

