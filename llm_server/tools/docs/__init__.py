# tools/docs/__init__.py
"""
Document processing toolkit for LLM applications.

This package provides model-agnostic utilities for handling documents, 
context management, and related operations across different LLM integrations.
"""

from tools.docs.chunking import chunk_text
from tools.docs.embedding_utils import safely_convert_embedding_to_list
from tools.docs.context_utils import retrieve_relevant_context, build_llm_context, prepare_context_for_llm
from tools.docs.message_utils import get_most_recent_user_query, store_assistant_response, process_incoming_messages
from tools.docs.attachment_utils import process_attachments_for_vectorization
from tools.docs.thread_utils import handle_thread_management

__all__ = [
    'chunk_text',
    'safely_convert_embedding_to_list',
    'retrieve_relevant_context',
    'build_llm_context',
    'prepare_context_for_llm',
    'get_most_recent_user_query',
    'store_assistant_response',
    'process_incoming_messages',
    'process_attachments_for_vectorization',
    'handle_thread_management',
] 