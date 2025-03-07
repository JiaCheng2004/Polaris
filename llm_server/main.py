from fastapi import FastAPI
import uvicorn

# Import the application factory from server/api
from server.service import create_app

# Create the FastAPI application
app: FastAPI = create_app()

if __name__ == "__main__":
    # Run Uvicorn server on port 3000
    uvicorn.run("main:app", host="0.0.0.0", port=3000, reload=True)
