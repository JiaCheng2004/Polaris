# tools/docs/thread_utils.py
"""
Thread management utilities for conversation threads.
These utilities are model-agnostic and can be used with any LLM integration.
"""

from typing import Dict, Any, Optional
from tools.database.thread.create import create_thread
from tools.database.thread.read import get_thread
from tools.logger import logger

def handle_thread_management(
    thread_id: Optional[str],
    model_name: str,
    provider: str,
    purpose: str,
    author: Dict[str, Any]
) -> str:
    """
    Handle thread creation or verification. Checks if a thread exists,
    and creates a new one if needed.
    
    Args:
        thread_id: Optional thread ID for continuing a conversation
        model_name: The model being used
        provider: The provider of the model
        purpose: The purpose of the thread
        author: Information about the author
        
    Returns:
        str: The thread ID to use
    """
    logger.info(f"Handling thread management. Thread ID provided: {thread_id}")
    
    # Check if thread exists
    if thread_id:
        try:
            existing_thread = get_thread(thread_id)
            logger.info(f"Found existing thread: {thread_id}")
            return thread_id
        except Exception as e:
            logger.error(f"Error finding thread {thread_id}: {str(e)}")
            logger.info("Creating new thread since provided thread_id was not found")
    
    # Create new thread
    try:
        thread_data = create_thread(
            model=model_name,
            provider=provider,
            purpose=purpose,
            author=author
        )
        new_thread_id = thread_data["thread_id"]
        logger.info(f"Created new thread: {new_thread_id}")
        return new_thread_id
    except Exception as e:
        logger.error(f"Error creating thread: {str(e)}")
        raise 