# server/api/v1/status.py

import time
import psutil
from fastapi import APIRouter, Request
from tools.matrics import update_memory_usage, increment_request_count

router = APIRouter()

def _human_readable_time(seconds: float) -> str:
    """
    Convert a number of seconds into a human-readable string of days, hours,
    minutes, and seconds.
    """
    days, seconds = divmod(seconds, 86400)
    hours, seconds = divmod(seconds, 3600)
    minutes, seconds = divmod(seconds, 60)

    parts = []
    if days:
        parts.append(f"{int(days)}d")
    if hours:
        parts.append(f"{int(hours)}h")
    if minutes:
        parts.append(f"{int(minutes)}m")
    if seconds:
        parts.append(f"{int(seconds)}s")

    return " ".join(parts) if parts else "0s"

def _human_readable_bytes(num_bytes: float) -> str:
    """
    Convert a number of bytes to a human-readable string (e.g., KB, MB, GB).
    """
    for unit in ["B", "KB", "MB", "GB", "TB"]:
        if num_bytes < 1024:
            return f"{num_bytes:.2f} {unit}"
        num_bytes /= 1024
    return f"{num_bytes:.2f} TB"  # in case it's huge

@router.get("")
async def get_status(request: Request):
    """
    Returns extended server status:
      - Uptime in human-readable format
      - Memory usage (total, used, percentage)
      - CPU load (optional)
      - Overall health
    """
    # Calculate uptime from app.state.start_time (set in create_app)
    current_time = time.time()
    start_time = request.app.state.start_time
    uptime_seconds = current_time - start_time
    uptime_str = _human_readable_time(uptime_seconds)

    # Memory usage using psutil
    mem = psutil.virtual_memory()
    update_memory_usage(mem.used)
    total_mem_human = _human_readable_bytes(mem.total)
    used_mem_human = _human_readable_bytes(mem.used)
    mem_percent = mem.percent

    # CPU usage (last 1 second average)
    cpu_percent = psutil.cpu_percent(interval=None)

    increment_request_count()
    
    status_info = {
        "status": "OK",
        "uptime": uptime_str,
        "memory": {
            "total": total_mem_human,
            "used": used_mem_human,
            "percent_used": f"{mem_percent}%",
        },
        "cpu_usage": f"{cpu_percent}%",
        "timestamp": int(current_time),
    }

    return status_info
