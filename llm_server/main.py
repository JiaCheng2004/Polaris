from fastapi import FastAPI
import uvicorn

# Import the application factory from server/api
from server.service import create_app

# Create the FastAPI application
app: FastAPI = create_app()

if __name__ == "__main__":
    # Run Uvicorn server on port 8080 with proper configuration
    uvicorn.run(
        "main:app", 
        host="0.0.0.0", 
        port=8080, 
        reload=True,
        # Configure Uvicorn to handle trailing slashes correctly
        forwarded_allow_ips="*",
        proxy_headers=True
    )
