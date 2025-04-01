# Document Processing Tools

This toolkit provides model-agnostic utilities for handling documents, context management, and conversation handling across different LLM integrations.

## Key Features

- **Document Chunking**: Split large documents into manageable chunks with configurable sizes and overlaps
- **Embedding Utilities**: Handle embeddings from various models and conversion formats
- **Context Management**: Retrieve and optimize context for different LLM models
- **Thread Management**: Handle conversation threads with support for memory and history
- **Message Processing**: Process and store messages with vectorization for retrieval
- **Attachment Handling**: Process file attachments, extract content, and vectorize for retrieval

## File Structure

- `__init__.py` - Package exports and documentation
- `attachment_utils.py` - Functions for processing and vectorizing file attachments
- `chunking.py` - Text chunking utilities for large documents
- `context_utils.py` - Context preparation and management functions
- `embedding_utils.py` - Embedding conversion and handling utilities
- `message_utils.py` - Message processing and storage utilities
- `thread_utils.py` - Thread management functions

## Usage

These utilities are designed to be model-agnostic and can be used with any LLM integration. For example:

```python
from tools.docs.context_utils import prepare_context_for_llm
from tools.docs.chunking import chunk_text
from tools.docs.message_utils import process_incoming_messages

# Example for a specific model implementation
def create_model_completion(payload, files):
    # Thread management
    thread_id = handle_thread_management(
        thread_id=payload.get("thread_id"),
        model_name="your-model-name",
        provider="your-provider",
        purpose="chat",
        author=payload.get("author")
    )
    
    # Process messages
    messages = process_incoming_messages(
        thread_id=thread_id,
        messages=payload.get("messages", []),
        author=payload.get("author"),
        embedding_model={"type": "your-embedding-model", "model": "your-model-name"}
    )
    
    # Get query and prepare context
    query_message = get_most_recent_user_query(messages)
    query_text, query_context, local_context = prepare_context_for_llm(
        thread_id=thread_id,
        query_message=query_message,
        max_tokens=32000,  # Model-specific token limit
        provider="your-provider",
        model="your-model-name"
    )
    
    # Generate response using model-specific client code
    response = your_model_client.generate(query_text, query_context, local_context)
    
    # Store response
    store_assistant_response(
        thread_id=thread_id,
        content=response,
        user_author=payload.get("author"),
        embedding_model={"type": "your-embedding-model", "model": "your-model-name"}
    )
    
    return {"thread_id": thread_id, "content": response}
```

## Customization

Each utility function provides parameters to customize behavior for different models:

- Specify embedding models and vector namespaces
- Set token limits based on model capabilities
- Configure chunking parameters
- Enable/disable summarization based on model availability
- Adjust weighting for different context components 