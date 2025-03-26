# tools/config/load.py

import os
import json
from pathlib import Path

CONFIG_PATH = Path(__file__).parents[2] / "config" / "config.json"

with open(CONFIG_PATH, "r") as f:
    CONFIG = json.load(f)

SERVER = CONFIG["server"]
VERSION = CONFIG["version"]
DEBUG = CONFIG["debug"]

# Environmental variables:

POSTGREST_BASE_URL = os.getenv("POSTGREST_BASE_URL")
OPENAI_API_KEY = os.getenv("OPENAI_API_KEY")
GOOGLE_API_KEY = os.getenv("GOOGLE_API_KEY")
ANTHROPIC_API_KEY = os.getenv("ANTHROPIC_API_KEY")
XAI_API_KEY = os.getenv("XAI_API_KEY")
ZHIPU_API_KEY = os.getenv("ZHIPU_API_KEY")
LLAMA_API_KEY = os.getenv("LLAMA_API_KEY")
DEEPSEEK_API_KEY = os.getenv("DEEPSEEK_API_KEY")
OPENAI_API_KEY = os.getenv("OPENAI_API_KEY")
POSTGREST_JWT_SECRET = os.getenv("POSTGREST_JWT_SECRET")
VOLCENGINE_ACCESSKEYID = os.getenv("VOLCENGINE_ACCESSKEYID")
VOLCENGINE_SECRET_ACCESSKEY = os.getenv("VOLCENGINE_SECRET_ACCESSKEY")
FEATHERLESS_API_KEY = os.getenv("FEATHERLESS_API_KEY")
TOGETHER_API_KEY = os.getenv("TOGETHER_API_KEY")
