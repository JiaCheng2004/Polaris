# tools/docs/context_utils.py
"""
Context management utilities for retrieving and optimizing context for LLMs.
These utilities are model-agnostic and can be used with any LLM integration.
"""

from typing import Dict, List, Tuple, Any, Optional
import traceback

from tools.database.vector.read import search_vectors
from tools.database.file import get_file
from tools.embed.text import embed_text
from tools.llm.top_k_selector import get_optimal_top_k
from tools.llm.summarizer import summarize_context
from tools.tokenizer import token_counter
from tools.logger import logger

def retrieve_relevant_context(
    thread_id: str, 
    query_text: str,
    namespace: str = "files",
    similarity_threshold: float = 0.5,
    top_k: Optional[int] = None,
    use_optimal_k: bool = True
) -> str:
    """
    Retrieve relevant context from vector store based on query.
    
    Args:
        thread_id: The thread ID
        query_text: The query text
        namespace: Vector namespace to search in
        similarity_threshold: Minimum similarity threshold for results
        top_k: Number of results to return (if None, uses optimal or 5)
        use_optimal_k: Whether to compute optimal k based on query
        
    Returns:
        str: Relevant context as a string
    """
    logger.info(f"Retrieving relevant context for thread {thread_id}")
    
    # Skip empty queries
    if not query_text or query_text.strip() == "":
        logger.warning("Empty query text provided, skipping context retrieval")
        return ""
    
    # Generate embedding for query
    try:
        embedding_list = embed_text(query_text)
        if not embedding_list:
            logger.warning("Embedding function returned None for query")
            return ""
    except Exception as e:
        logger.error(f"Error generating embedding for query: {str(e)}")
        return ""
    
    # Log embedding info
    logger.debug(f"Query embedding type: {type(embedding_list).__name__}, length: {len(embedding_list)}")
    
    # Get optimal top_k based on query
    limit = top_k or 5
    if use_optimal_k and not top_k:
        try:
            top_k_result = get_optimal_top_k(query_text)
            limit = top_k_result.get("top_k", 5)
            logger.info(f"Using optimal top_k = {limit} for context retrieval")
        except Exception as e:
            logger.warning(f"Error getting optimal top_k, using default: {str(e)}")
    
    # Search for relevant vectors with better error handling
    try:
        similar_vectors = search_vectors(
            query_embedding=embedding_list,
            namespace=namespace,
            thread_id=thread_id,
            similarity_threshold=similarity_threshold,
            limit=limit
        )
        
        # Check if we got any vectors back
        if not similar_vectors:
            logger.info("No similar vectors found in database")
            return ""
            
        # Format retrieved contexts
        contexts = []
        for i, vector in enumerate(similar_vectors):
            content = vector.get("content", "")
            if content:
                source_info = ""
                metadata = vector.get("metadata", {})
                if metadata:
                    file_name = metadata.get("file_name", "")
                    if file_name:
                        source_info = f" (Source: {file_name})"
                
                contexts.append(f"Chunk #{i+1}{source_info}:\n{content}")
        
        logger.info(f"Retrieved {len(contexts)} relevant context chunks")
        return "\n\n".join(contexts)
    except Exception as e:
        logger.error(f"Error retrieving context: {str(e)}")
        logger.error(f"Traceback: {traceback.format_exc()}")
        return ""

def build_llm_context(
    query_text: str,
    query_context: str,
    local_context: str,
    max_tokens: int,
    provider: str,
    model: str,
    weighting: Optional[Dict[str, int]] = None,
    use_summarization: bool = True
) -> Tuple[str, str, str]:
    """
    Build the context for the LLM, ensuring it fits within token limits.
    
    Args:
        query_text: The user query
        query_context: Context from query's attachments
        local_context: Retrieved context from vector store
        max_tokens: Maximum token limit
        provider: The model provider
        model: The model name
        weighting: Priority weighting for each context type
            Example: {"query": 2, "query_context": 2, "local_context": 2}
        use_summarization: Whether to use summarization for reducing context
        
    Returns:
        Tuple[str, str, str]: Finalized query_text, query_context, local_context
    """
    logger.info("Building LLM context with token management")
    
    # Set default weighting if not provided
    if weighting is None:
        weighting = {
            "query": 2,  # Query priority
            "query_context": 2,  # Query context priority
            "local_context": 2  # Local context priority
        }
    
    # Extract priority weights
    p_A = weighting.get("query", 2)
    p_B = weighting.get("query_context", 2)
    p_C = weighting.get("local_context", 2)
    
    # Count tokens for each component
    query_text_tokens, _, _ = token_counter(query_text, provider, model)
    query_context_tokens, _, _ = token_counter(query_context, provider, model)
    local_context_tokens, _, _ = token_counter(local_context, provider, model)
    
    # Log token counts for each component
    logger.debug("============ TOKEN COUNTS FOR CONTEXT COMPONENTS ============")
    logger.debug(f"Query Text: {query_text_tokens} tokens")
    logger.debug(f"Query Context: {query_context_tokens} tokens")
    logger.debug(f"Local Context: {local_context_tokens} tokens")
    logger.debug(f"Total: {query_text_tokens + query_context_tokens + local_context_tokens} tokens (max: {max_tokens})")
    logger.debug("===========================================================")
    
    # Quick check if everything fits
    total_tokens = query_text_tokens + query_context_tokens + local_context_tokens
    if total_tokens <= max_tokens:
        logger.info(f"All content fits within token limit ({total_tokens}/{max_tokens})")
        return query_text, query_context, local_context
    
    logger.warning(f"Total tokens {total_tokens} exceeds limit {max_tokens}. Need to reduce content.")
    
    # If query alone exceeds the limit, summarize it
    if query_text_tokens > max_tokens:
        logger.info(f"Query text exceeds limit, summarizing ({query_text_tokens}/{max_tokens})")
        if use_summarization:
            result = summarize_context(query_text, max_tokens, provider, model)
            if result["status"] == 200:
                query_text = result["content"]
                logger.debug(f"Summarized query from {query_text_tokens} to {token_counter(query_text, provider, model)[0]} tokens")
                return query_text, "", ""
        else:
            # Simple truncation if summarization is disabled
            truncation_ratio = max_tokens / query_text_tokens
            query_text = query_text[:int(len(query_text) * truncation_ratio)]
            logger.debug(f"Truncated query to approximately {max_tokens} tokens")
            return query_text, "", ""
    
    # Calculate weighting ratio
    W = p_A + p_B + p_C
    
    # Calculate capacity slices
    c_A = (p_A / W) * max_tokens
    c_B = (p_B / W) * max_tokens
    c_C = (p_C / W) * max_tokens
    
    logger.debug(f"Token allocation - Query: {int(c_A)}, Query Context: {int(c_B)}, Local Context: {int(c_C)}")
    
    # Try to fit query fully
    if query_text_tokens <= c_A:
        final_query = query_text
        leftover = c_A - query_text_tokens
        c_B += leftover
        logger.debug(f"Query fits within allocation. Leftover {int(leftover)} tokens added to Query Context (now {int(c_B)})")
    else:
        # Summarize or truncate query
        logger.info(f"Query exceeds its allocation, handling ({query_text_tokens}/{int(c_A)})")
        if use_summarization:
            result = summarize_context(query_text, int(c_A), provider, model)
            if result["status"] == 200:
                final_query = result["content"]
                logger.debug(f"Summarized query from {query_text_tokens} to {token_counter(final_query, provider, model)[0]} tokens")
            else:
                # Fallback if summarization fails
                final_query = query_text[:int(len(query_text) * (c_A / query_text_tokens))]
                logger.warning(f"Summarization failed, truncated query to {token_counter(final_query, provider, model)[0]} tokens")
        else:
            final_query = query_text[:int(len(query_text) * (c_A / query_text_tokens))]
            logger.debug(f"Truncated query to {token_counter(final_query, provider, model)[0]} tokens")
    
    # Try to fit query context fully
    if query_context_tokens <= c_B:
        final_query_context = query_context
        leftover = c_B - query_context_tokens
        c_C += leftover
        logger.debug(f"Query context fits within allocation. Leftover {int(leftover)} tokens added to Local Context (now {int(c_C)})")
    else:
        # Summarize or truncate query context
        logger.info(f"Query context exceeds its allocation, handling ({query_context_tokens}/{int(c_B)})")
        if use_summarization:
            result = summarize_context(query_context, int(c_B), provider, model)
            if result["status"] == 200:
                final_query_context = result["content"]
                logger.debug(f"Summarized query context from {query_context_tokens} to {token_counter(final_query_context, provider, model)[0]} tokens")
            else:
                # Fallback if summarization fails
                final_query_context = query_context[:int(len(query_context) * (c_B / query_context_tokens))]
                logger.warning(f"Summarization failed, truncated query context to {token_counter(final_query_context, provider, model)[0]} tokens")
        else:
            final_query_context = query_context[:int(len(query_context) * (c_B / query_context_tokens))]
            logger.debug(f"Truncated query context to {token_counter(final_query_context, provider, model)[0]} tokens")
    
    # Fit local context
    if local_context_tokens <= c_C:
        final_local_context = local_context
        logger.debug(f"Local context fits within allocation ({local_context_tokens}/{int(c_C)})")
    else:
        # Summarize or truncate local context
        logger.info(f"Local context exceeds its allocation, handling ({local_context_tokens}/{int(c_C)})")
        if use_summarization:
            result = summarize_context(local_context, int(c_C), provider, model)
            if result["status"] == 200:
                final_local_context = result["content"]
                logger.debug(f"Summarized local context from {local_context_tokens} to {token_counter(final_local_context, provider, model)[0]} tokens")
            else:
                # Fallback if summarization fails
                final_local_context = local_context[:int(len(local_context) * (c_C / local_context_tokens))]
                logger.warning(f"Summarization failed, truncated local context to {token_counter(final_local_context, provider, model)[0]} tokens")
        else:
            final_local_context = local_context[:int(len(local_context) * (c_C / local_context_tokens))]
            logger.debug(f"Truncated local context to {token_counter(final_local_context, provider, model)[0]} tokens")
    
    # Final token check
    final_total_tokens = token_counter(final_query, provider, model)[0] + token_counter(final_query_context, provider, model)[0] + token_counter(final_local_context, provider, model)[0]
    logger.info(f"Final context size after adjustments: {final_total_tokens}/{max_tokens} tokens")
    
    logger.info("Context building complete")
    return final_query, final_query_context, final_local_context

def prepare_context_for_llm(
    thread_id: str, 
    query_message: Dict[str, Any],
    max_tokens: Optional[int] = None,
    provider: str = "default",
    model: str = "default", 
    vector_namespace: str = "files",
    use_summarization: bool = True,
    context_weighting: Optional[Dict[str, int]] = None
) -> Tuple[str, str, str]:
    """
    Prepare context for the LLM including query text, query context, and local context.
    
    Args:
        thread_id: The thread ID
        query_message: The query message object
        max_tokens: Maximum token limit (model-specific)
        provider: The model provider
        model: The model name
        vector_namespace: The namespace to search for relevant context
        use_summarization: Whether to use summarization for reducing context
        context_weighting: Priority weighting for each context type
            Example: {"query": 2, "query_context": 2, "local_context": 2}
        
    Returns:
        Tuple[str, str, str]: query_text, query_context_text, local_context_text
    """
    logger.info(f"Preparing context for LLM with query message {query_message.get('message_id', 'unknown')}")
    
    # Set default max tokens if not provided
    if max_tokens is None:
        # This is a reasonable default for many models, but should be overridden
        # by the specific model implementation
        max_tokens = 16000
    
    # Extract query text from the message
    query_text = ""
    if query_message.get("content", {}).get("type") == "text":
        query_text = query_message.get("content", {}).get("text", "")
    
    # Get query attachments content
    query_attachments = []
    file_ids = query_message.get("file_ids", [])
    
    for file_id in file_ids:
        try:
            file_data = get_file(file_id)
            if file_data and "content" in file_data:
                query_attachments.append(file_data["content"])
        except Exception as e:
            logger.error(f"Error retrieving file {file_id}: {str(e)}")
    
    query_context_text = "\n".join(query_attachments)
    
    # Retrieve relevant local context using embeddings
    local_context_text = retrieve_relevant_context(
        thread_id=thread_id, 
        query_text=query_text,
        namespace=vector_namespace
    )
    
    # Log the original context components before any modification
    logger.debug("============ ORIGINAL CONTEXT COMPONENTS ============")
    logger.debug(f"Query Text (first 500 chars): {query_text[:500]}")
    logger.debug(f"Query Context (first 500 chars): {query_context_text[:500]}")
    logger.debug(f"Local Context (first 500 chars): {local_context_text[:500]}")
    logger.debug("===================================================")
    
    # Ensure everything fits within token limits
    query_text, query_context_text, local_context_text = build_llm_context(
        query_text=query_text,
        query_context=query_context_text,
        local_context=local_context_text,
        max_tokens=max_tokens,
        provider=provider,
        model=model,
        weighting=context_weighting,
        use_summarization=use_summarization
    )
    
    # Log the final context components after token fitting
    logger.debug("============ FINAL CONTEXT COMPONENTS AFTER TOKEN FITTING ============")
    logger.debug(f"Final Query Text (first 500 chars): {query_text[:500]}")
    logger.debug(f"Final Query Context (first 500 chars): {query_context_text[:500]}")
    logger.debug(f"Final Local Context (first 500 chars): {local_context_text[:500]}")
    logger.debug("===================================================================")
    
    return query_text, query_context_text, local_context_text 