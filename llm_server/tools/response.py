# tools/response.py

"""
Custom Response and error handling utilities.
"""
from fastapi.responses import JSONResponse

def custom_response(data, status_code=200):
    return JSONResponse(
        content={"data": data},
        status_code=status_code
    )
