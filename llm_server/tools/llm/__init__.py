# tools/llm/__init__.py
from .top_k_selector import get_optimal_top_k as top_k
from .summarizer import summarize_context as summarize
from .search_indicator import detect_search_needs as search_needs