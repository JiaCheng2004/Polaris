import os
import importlib.util

def token_counter(text, provider, model):
    """
    Count tokens using the specified provider and model.
    
    Args:
        text (str): The text to tokenize
        provider (str): The provider name (e.g., 'google', 'openai', 'deepseek')
        model (str): The model name within that provider
        
    Returns:
        tuple: (token_count, error_code, error_message)
            - If successful: (token_count, None, None)
            - If provider not found: (0, 404, "Provider not found" message)
            - If model not found: (0, 404, "Model not found" message)
    """
    # Check if provider exists
    provider_dir = os.path.join(os.path.dirname(__file__), provider)
    if not os.path.isdir(provider_dir):
        return 0, 404, "Requested provider not found. Verify the provider name and try again."
    
    # Check if model exists
    model_file = os.path.join(provider_dir, f"{model}.py")
    if not os.path.isfile(model_file):
        return 0, 404, "Requested model not found within the specified provider. Please verify the model name and provider."
    
    # Load the module dynamically
    module_name = f"tools.tokenizer.{provider}.{model}"
    spec = importlib.util.spec_from_file_location(module_name, model_file)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    
    # Call the tokenize function from the module
    if hasattr(module, 'tokenize'):
        try:
            token_count = module.tokenize(text)
            return token_count, None, None
        except Exception as e:
            # Return a fallback estimate if the tokenizer fails
            fallback_count = len(text) // 4  # Simple approximation
            return fallback_count, None, None
    else:
        return 0, 500, f"The module {module_name} does not have a tokenize function." 