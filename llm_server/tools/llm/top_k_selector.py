# tools/llm/top_k_selector.py

import json
from pydantic import BaseModel, Field
from together import Together

# Import the TOGETHER_API_KEY from the load config
from tools.config.load import TOGETHER_API_KEY

# Initialize the Together client with the API key
client = Together(api_key=TOGETHER_API_KEY)

class TopKResponse(BaseModel):
    """
    Pydantic model that describes the JSON response schema
    for the optimal number of chunks (top_k).
    """
    top_k: int = Field(description="The optimal number of chunks to retrieve (3, 5, or 8)")

def get_optimal_top_k(query_text: str) -> dict:
    """
    Determines the optimal number of chunks (top_k) to retrieve from a vector store
    based on the specificity of the user query.
    
    Args:
        query_text (str): The user query text to analyze
        
    Returns:
        dict: A dictionary with the key 'top_k' and the value being 3, 5, or 8
    """
    # Make the request to the LLM with the specified JSON schema
    response = client.chat.completions.create(
        messages=[
            {
                "role": "system",
                "content": """You are an expert at choosing the optimal number of chunks (top_k) to retrieve from a vector store for a given user query.
Based on the user's query, determine how specific or broad it is, and select the appropriate top_k value:

- Pick 3 if very specific and focused.
- Pick 5 if moderately specific.
- Pick 8 if very broad or open ended.

Return only a JSON object with the 'top_k' key and appropriate value."""
            },
            {
                "role": "user",
                "content": query_text,
            },
        ],
        model="deepseek-ai/DeepSeek-V3",
        response_format={
            "type": "json_object",
            "schema": TopKResponse.model_json_schema(),
        },
    )
    
    # Parse the LLM response to extract the JSON data
    result = json.loads(response.choices[0].message.content)
    return result
