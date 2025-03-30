# tools/embed/text.py

from typing import List, Optional, Dict, Any, Union
from google import genai
from google.genai import types
from ..config.load import GOOGLE_API_KEY
from ..logger import logger

# Initialize the Google Generative AI client
try:
    client = genai.Client(api_key=GOOGLE_API_KEY)
except Exception as e:
    logger.error(f"Failed to initialize Google Generative AI client: {e}")
    client = None

def embed_text(
    text: str, 
    model: str = "gemini-embedding-exp-03-07",
    dimensions: Optional[int] = None
) -> Union[List[float], None]:
    """
    Generate embeddings for a given text using Google's Gemini model.
    Ensures that either a List[float] or None is returned.
    Assumes input is under 8,192 tokens maximum.
    
    Args:
        text (str): The text to embed.
        model (str): The embedding model to use.
        dimensions (Optional[int]): Number of dimensions to truncate to.
    
    Returns:
        Union[List[float], None]: A list of floats or None if embedding failed.
    """
    if not client:
        logger.error("Google Generative AI client not initialized")
        return None
    
    if not text:
        logger.warning("Empty text provided for embedding")
        return None
    
    try:
        # Generate embedding
        logger.debug(f"Generating embedding for text (length {len(text)}) with model {model}")
        result = client.models.embed_content(
            model=model,
            contents=text,
            config=types.EmbedContentConfig(task_type="SEMANTIC_SIMILARITY")
        )
        
        # Extract the embedding values directly
        embedding_list = result.embeddings[0].values
        
        # Truncate dimensions if needed
        if dimensions and dimensions > 0 and dimensions < len(embedding_list):
            logger.debug(f"Truncating embedding from {len(embedding_list)} to {dimensions} dimensions")
            embedding_list = embedding_list[:dimensions]
            
        return embedding_list
        
    except Exception as e:
        logger.error(f"Fatal error during embedding generation: {str(e)}")
        import traceback
        logger.error(f"Embedding error traceback: {traceback.format_exc()}")
        return None 