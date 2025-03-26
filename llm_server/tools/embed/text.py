# tools/embed/text.py

from typing import List, Optional, Dict, Any, Union
from google import genai
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
    
    Args:
        text (str): The text to embed. Maximum length is 8K tokens.
        model (str): The embedding model to use. Default is "gemini-embedding-exp-03-07".
        dimensions (Optional[int]): Number of dimensions to truncate to (MRL feature).
                                    Default is None, which gives the full 3K dimensions.
    
    Returns:
        Union[List[float], None]: A list of floating point numbers representing the text embedding,
                                  or None if embedding failed.
    
    Features:
        - Input token limit of 8K tokens. Embeds large chunks of text, code, or other data.
        - Output dimensions of 3K dimensions (can be truncated using the dimensions parameter).
        - Matryoshka Representation Learning (MRL) allows for truncating dimensions.
        - Supports over 100 languages.
        - Unified model that handles multiple tasks and languages.
    """
    if not client:
        logger.error("Google Generative AI client not initialized")
        return None
    
    if not text:
        logger.warning("Empty text provided for embedding")
        return None
    
    try:
        # Generate embedding
        result = client.models.embed_content(
            model=model,
            contents=text
        )
        
        # Get the embedding values
        embeddings = result.embeddings
        
        # Truncate dimensions if specified (MRL feature)
        if dimensions and dimensions > 0 and dimensions < len(embeddings):
            return embeddings[:dimensions]
        
        return embeddings
    except Exception as e:
        logger.error(f"Error generating embeddings: {e}")
        return None 