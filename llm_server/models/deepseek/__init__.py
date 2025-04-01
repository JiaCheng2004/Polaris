# models/deepseek/__init__.py

"""
DeepSeek model integration package.

This package provides integration with DeepSeek's LLM models using the
model-agnostic document toolkit.
"""

from .deepseek_reasoner import create_deepseek_reasoner_completion
from .deepseek_chat import create_deepseek_chat_completion

__all__ = [
    "create_deepseek_reasoner_completion",
    "create_deepseek_chat_completion"
]
