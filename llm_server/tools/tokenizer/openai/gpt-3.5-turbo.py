import tiktoken

def tokenize(text):
    """
    Count tokens for OpenAI GPT-3.5 Turbo model using tiktoken
    
    Args:
        text: The text to tokenize
        
    Returns:
        int: Number of tokens in the text
    """
    if not text:
        return 0
        
    try:
        encoding = tiktoken.encoding_for_model("gpt-3.5-turbo")
        tokens = encoding.encode(text)
        return len(tokens)
    except Exception as e:
        # Log the error and raise it to be handled by the caller
        print(f"Error counting tokens with tiktoken for gpt-3.5-turbo: {str(e)}")
        raise 