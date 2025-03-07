# tools/logger.py

import logging

logger = logging.getLogger("llm_server_logger")
logger.setLevel(logging.INFO)

# You can add formatting, handlers, etc., here
console_handler = logging.StreamHandler()
formatter = logging.Formatter("%(asctime)s - %(levelname)s - %(message)s")
console_handler.setFormatter(formatter)
logger.addHandler(console_handler)
