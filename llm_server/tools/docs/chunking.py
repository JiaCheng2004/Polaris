# tools/docs/chunking.py
"""
Text chunking utilities for processing large documents.
These utilities are model-agnostic and can be used with any LLM integration.
"""

from typing import List
from tools.logger import logger

def chunk_text(text: str, chunk_size: int, chunk_overlap: int) -> List[str]:
    """
    Split text into overlapping chunks using LangChain's RecursiveCharacterTextSplitter.
    
    Args:
        text: The text to split
        chunk_size: Maximum size of each chunk (in characters)
        chunk_overlap: Overlap between chunks (in characters)
    
    Returns:
        List[str]: List of text chunks
    """
    try:
        # Import langchain text splitter
        from langchain_text_splitters import RecursiveCharacterTextSplitter
        
        logger.info(f"Using LangChain's RecursiveCharacterTextSplitter with chunk_size={chunk_size}, chunk_overlap={chunk_overlap}")
        
        # Create text splitter with appropriate parameters
        text_splitter = RecursiveCharacterTextSplitter(
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
            length_function=len,
            is_separator_regex=False,
            separators=["\n\n", "\n", " ", ""]
        )
        
        # Split text
        chunks = text_splitter.split_text(text)
        
        # Log chunk information
        logger.info(f"Split text into {len(chunks)} chunks")
        chunk_lengths = [len(chunk) for chunk in chunks]
        if chunk_lengths:
            logger.debug(f"Chunk lengths - min: {min(chunk_lengths)}, max: {max(chunk_lengths)}, avg: {sum(chunk_lengths)/len(chunk_lengths):.1f}")
        
        return chunks
    except ImportError as e:
        # Fallback to original implementation if LangChain is not available
        logger.warning(f"LangChain import error: {str(e)}. Falling back to basic chunking method.")
        return _chunk_text_basic(text, chunk_size, chunk_overlap)
    except Exception as e:
        # Fallback if there are any other errors
        logger.error(f"Error using LangChain text splitter: {str(e)}. Falling back to basic chunking method.")
        import traceback
        logger.error(f"Traceback: {traceback.format_exc()}")
        return _chunk_text_basic(text, chunk_size, chunk_overlap)

def _chunk_text_basic(text: str, chunk_size: int, chunk_overlap: int) -> List[str]:
    """
    Basic fallback method to split text into overlapping chunks.
    
    Args:
        text: The text to split
        chunk_size: Maximum size of each chunk
        chunk_overlap: Overlap between chunks
    
    Returns:
        List[str]: List of text chunks
    """
    logger.info("Using basic chunking method")
    chunks = []
    start = 0
    text_length = len(text)
    
    while start < text_length:
        # Calculate end position with respect to text length
        end = min(start + chunk_size, text_length)
        
        # Try to end at a paragraph or sentence if possible
        if end < text_length:
            # Look for paragraph breaks first
            paragraph_end = text.rfind("\n\n", start, end)
            if paragraph_end != -1 and paragraph_end > start + chunk_size // 2:  # Ensure meaningful chunk size
                end = paragraph_end + 2  # Include the newlines
            else:
                # Look for sentence breaks
                sentence_end = text.rfind(". ", start, end)
                if sentence_end != -1 and sentence_end > start + chunk_size // 3:  # Ensure meaningful chunk size
                    end = sentence_end + 2  # Include the period and space
        
        # Extract chunk
        chunk = text[start:end]
        chunks.append(chunk)
        
        # Move starting position for next chunk, considering overlap
        start = start + chunk_size - chunk_overlap
        
        # Break if we've reached the end
        if start >= text_length:
            break
    
    # Log chunk information
    logger.info(f"Split text into {len(chunks)} chunks using basic method")
    chunk_lengths = [len(chunk) for chunk in chunks]
    if chunk_lengths:
        logger.debug(f"Chunk lengths - min: {min(chunk_lengths)}, max: {max(chunk_lengths)}, avg: {sum(chunk_lengths)/len(chunk_lengths):.1f}")
    
    return chunks 