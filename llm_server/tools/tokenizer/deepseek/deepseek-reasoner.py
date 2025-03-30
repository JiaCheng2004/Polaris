from deepseek_tokenizer import ds_token

def tokenize(text):
    """
    Count tokens for Deepseek Reasoner model
    
    Args:
        text: The text to tokenize
        
    Returns:
        int: Number of tokens in the text
    """
    if not text:
        return 0
        
    try:
        tokens = ds_token.encode(text)
        return len(tokens)
    except Exception as e:
        # Log the error and raise it to be handled by the caller
        print(f"Error counting tokens with Deepseek tokenizer: {str(e)}")
        raise 