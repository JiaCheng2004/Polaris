# models/deepseek/__init__.py

from .deepseek_chat import create_deepseek_chat_completion
from .deepseek_reasoner import create_deepseek_reasoner_completion

__all__ = ['create_deepseek_chat_completion', 'create_deepseek_reasoner_completion']
