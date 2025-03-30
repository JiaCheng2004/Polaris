# tools/llm/summarizer.py

import json
from google import genai
from google.genai.types import HttpOptions
from tools.config.load import GOOGLE_API_KEY
from tools.tokenizer import token_counter

MAX_TOKEN_LIMIT = 1_000_000

class Summarizer:
    def __init__(self):
        """Initialize the Summarizer tool."""
        self.gemini_client = genai.Client(
            http_options=HttpOptions(api_version="v1"), 
            api_key=GOOGLE_API_KEY
        )
    
    def _count_tokens(self, context, provider, model):
        """Count tokens using the specified provider and model."""
        token_count, error_code, error_message = token_counter(context, provider, model)
        return token_count, error_code, error_message
    
    def summarize(self, context, target_size, provider, model):
        """
        Summarize the context to be under the target token size.
        
        Args:
            context (str): The text to summarize
            target_size (int): Target token count to achieve
            provider (str): The token counting provider (e.g., 'google', 'openai', 'deepseek')
            model (str): The specific model within the provider
            
        Returns:
            dict: Result with status, message, content, and token sizes
        """
        # Handle empty context
        if not context or context.strip() == "":
            original_size, error_code, error_message = self._count_tokens(context, provider, model)
            if error_code:
                return {
                    "status": error_code,
                    "message": error_message,
                    "content": "",
                    "original_size": 0,
                    "reduced_size": 0
                }
            
            return {
                "status": 204,
                "message": "Provided context is empty; no summarization necessary.",
                "content": "",
                "original_size": original_size,
                "reduced_size": original_size
            }
        
        # Check token size of the original context
        original_size, error_code, error_message = self._count_tokens(context, provider, model)
        if error_code:
            return {
                "status": error_code,
                "message": error_message,
                "content": "",
                "original_size": 0,
                "reduced_size": 0
            }
        
        # If context exceeds maximum limit
        if original_size > MAX_TOKEN_LIMIT:
            return {
                "status": 400,
                "message": f"Context exceeds the maximum limit of {MAX_TOKEN_LIMIT:,} tokens. Please reduce the input size.",
                "content": "",
                "original_size": original_size,
                "reduced_size": 0
            }
        
        # If target size is already met, no need to summarize
        if original_size <= target_size:
            return {
                "status": 200,
                "message": "Context already meets the target size requirement.",
                "content": context,
                "original_size": original_size,
                "reduced_size": original_size
            }
        
        # Perform summarization
        current_context = context
        current_size = original_size
        max_attempts = 3
        
        for attempt in range(max_attempts):
            try:
                system_prompt = f"Summarize the provided context from current {current_size} to {target_size} by distilling all important details, relationships, and nuances into a concise and cohesive summary. Ensure that no vital information is omitted while being concise and clear."
                
                generation_config = {
                    "temperature": 0.2,
                    "top_p": 0.95,
                    "top_k": 40,
                    "max_output_tokens": min(target_size, 8192)
                }
                
                gemini_model = self.gemini_client.get_model("gemini-2.0-flash-001")
                response = gemini_model.generate_content(
                    contents=[{"role": "system", "parts": [system_prompt]},
                              {"role": "user", "parts": [current_context]}],
                    generation_config=generation_config
                )
                
                summarized_text = response.text
                current_size, error_code, error_message = self._count_tokens(summarized_text, provider, model)
                
                if error_code:
                    return {
                        "status": error_code,
                        "message": error_message,
                        "content": "",
                        "original_size": original_size,
                        "reduced_size": 0
                    }
                
                # If we've reached the target size, return the result
                if current_size <= target_size:
                    return {
                        "status": 200,
                        "message": "Successfully summarized the context.",
                        "content": summarized_text,
                        "original_size": original_size,
                        "reduced_size": current_size
                    }
                
                # Continue with the loop using the partially summarized text
                current_context = summarized_text
                
            except Exception as e:
                return {
                    "status": 500,
                    "message": f"Internal server error occurred: {str(e)}. Please try again later.",
                    "content": "",
                    "original_size": original_size,
                    "reduced_size": 0
                }
        
        # If we've tried max_attempts and still haven't reached target size
        return {
            "status": 200,
            "message": f"Partially summarized context. Reduced from {original_size} to {current_size} tokens, but didn't reach target of {target_size}.",
            "content": current_context,
            "original_size": original_size,
            "reduced_size": current_size
        }

# Create a single instance to be used by external callers
summarizer = Summarizer()

def summarize_context(context, target_size, provider, model):
    """
    Interface function for summarizing context.
    
    Args:
        context (str): The context to summarize
        target_size (int): Target token size to reduce to
        provider (str): Provider for token counting method
        model (str): Model name within the provider
        
    Returns:
        dict: Result with status, message, content, original_size, and reduced_size
    """
    return summarizer.summarize(context, target_size, provider, model)

