# tools/docs/embedding_utils.py
"""
Embedding utilities to handle various embedding formats and conversions.
These utilities are model-agnostic and can be used with any embedding model.
"""

import logging
from typing import Any, List, Optional

# Import logger
from tools.logger import logger

def safely_convert_embedding_to_list(embedding: Any) -> Optional[List[float]]:
    """
    Safely convert various embedding types to a standard Python list.
    Handles ContentEmbedding and other non-standard embedding return types
    from various models and APIs.
    
    Args:
        embedding: The embedding to convert
        
    Returns:
        Optional[List[float]]: The embedding as a list, or None if conversion failed
    """
    if embedding is None:
        return None
        
    # If it's already a list, return it directly
    if isinstance(embedding, list):
        # Verify all elements are floats
        try:
            return [float(x) for x in embedding]
        except (TypeError, ValueError) as e:
            logger.error(f"List conversion error - Failed to convert embedding values to float: {str(e)}")
            return None
    
    # Log the embedding type for debugging
    logger.debug(f"Converting embedding of type: {type(embedding).__name__}, repr: {repr(embedding)[:100]}...")
        
    # Handle Google Gemini's ContentEmbedding type specifically
    if hasattr(embedding, 'values') and callable(getattr(embedding, 'values', None)):
        try:
            # If values is a method that returns an iterable
            logger.debug("Using .values() method")
            values = embedding.values()
            return [float(x) for x in values]
        except Exception as e:
            logger.error(f"Using .values() method failed: {str(e)}")
    
    # Handle the specific ContentEmbedding case (where values is a property, not a method)
    if hasattr(embedding, 'values') and not callable(getattr(embedding, 'values', None)):
        try:
            logger.debug("Using .values property")
            # Convert to list and ensure all elements are floats
            values_list = [float(x) for x in embedding.values]
            logger.debug(f"Successfully converted embedding using .values property, length: {len(values_list)}")
            return values_list
        except Exception as e:
            logger.error(f"Using .values property failed: {str(e)}")
    
    # Try accessing embeddings attribute (Gemini embed_content response)
    if hasattr(embedding, 'embeddings'):
        try:
            logger.debug("Using .embeddings attribute")
            return [float(x) for x in embedding.embeddings]
        except Exception as e:
            logger.error(f"Using .embeddings attribute failed: {str(e)}")
            
    # Try different conversion methods
    try:
        # Try direct list conversion
        list_values = list(embedding)
        float_values = [float(x) for x in list_values]
        logger.debug(f"Direct list conversion successful, length: {len(float_values)}")
        return float_values
    except (TypeError, ValueError) as e:
        logger.error(f"Direct list conversion failed: {str(e)}")
        
    try:
        # Try using the embedding's to_list method if available
        if hasattr(embedding, 'to_list'):
            logger.debug("Using to_list() method")
            return [float(x) for x in embedding.to_list()]
    except Exception as e:
        logger.error(f"to_list() method failed: {str(e)}")
        
    try:
        # Try converting via __iter__ if available (for iterator-like objects)
        if hasattr(embedding, '__iter__'):
            logger.debug("Using __iter__ method")
            return [float(x) for x in embedding]
    except Exception as e:
        logger.error(f"Iteration conversion failed: {str(e)}")
        
    try:
        # Try getting embedding as a dictionary and converting values
        if hasattr(embedding, '__dict__'):
            logger.debug("Trying to convert from __dict__ attribute")
            embedding_dict = embedding.__dict__
            if isinstance(embedding_dict, dict) and any(isinstance(v, (int, float)) for v in embedding_dict.values()):
                return [float(v) for v in embedding_dict.values() if isinstance(v, (int, float))]
    except Exception as e:
        logger.error(f"Dict conversion failed: {str(e)}")
        
    # If none of the conversion methods worked
    logger.error(f"Failed to convert embedding of type {type(embedding).__name__} to list")
    return None 