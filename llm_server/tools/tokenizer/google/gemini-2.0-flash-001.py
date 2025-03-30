from google import genai
from google.genai.types import HttpOptions
from tools.config.load import GOOGLE_API_KEY

def tokenize(text):
    """
    Count tokens for Google Gemini 2.0 Flash model
    
    Args:
        text: The text to tokenize
        
    Returns:
        int: Number of tokens in the text
    """
    if not text:
        return 0
        
    try:
        client = genai.Client(http_options=HttpOptions(api_version="v1"), api_key=GOOGLE_API_KEY)
        response = client.models.count_tokens(
            model="gemini-2.0-flash-001",
            contents=text,
        )
        return response.total_tokens
    except Exception as e:
        # Log the error and raise it to be handled by the caller
        print(f"Error counting tokens with Google Gemini 2.0 Flash model: {str(e)}")
        raise 