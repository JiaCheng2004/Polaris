import tiktoken

def tokenize(text):
    """
    Count tokens for OpenAI GPT-4 model using tiktoken
    
    Args:
        text: The text to tokenize
        
    Returns:
        int: Number of tokens in the text
    """
    if not text:
        return 0
        
    try:
        encoding = tiktoken.encoding_for_model("gpt-4")
        tokens = encoding.encode(text)
        return len(tokens)
    except Exception as e:
        # Log the error and raise it to be handled by the caller
        print(f"Error counting tokens with tiktoken for gpt-4: {str(e)}")
        raise 