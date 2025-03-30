# server/api/v1/metrics.py

from fastapi import APIRouter
from tools.matrics import get_current_metrics

router = APIRouter()

@router.get("")
async def get_metrics():
    """
    Return server and API usage metrics.
    """
    # Retrieve metrics from the tools.matrics module
    current_metrics = get_current_metrics()
    
    return {
        "metrics": current_metrics
    }
