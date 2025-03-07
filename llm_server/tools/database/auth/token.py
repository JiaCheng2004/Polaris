# tools/database/auth/token.py

import os
import jwt
import time
from tools.config.load import POSTGREST_JWT_SECRET

def generate_token() -> str:
    """
    Generates a JWT for the 'api' role
    """
    payload = {
        "role": "api",
        "iat": int(time.time()),
        "exp": int(time.time()) + 3600  
    }

    token = jwt.encode(payload, POSTGREST_JWT_SECRET, algorithm="HS256")
    return token
