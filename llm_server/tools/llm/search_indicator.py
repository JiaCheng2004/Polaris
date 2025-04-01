# tools/llm/search_indicator.py

import json
import os
from google import genai
from google.genai import types
from google.genai.types import HttpOptions
from pydantic import BaseModel, Field
from typing import List, Optional

# Import the GOOGLE_API_KEY from the load config
from tools.config.load import GOOGLE_API_KEY
from tools.logger import logger

class SearchIndicatorResponse(BaseModel):
    """
    Pydantic model that describes the JSON response schema
    for the search indicator tool.
    """
    tool: List[str] = Field(description="Array of tools to use: ['web_search', 'video', 'web_scrap']")
    web_search: Optional[str] = Field(description="The search query if web_search is needed", default=None)
    videos: Optional[List[str]] = Field(description="List of video URLs if video tool is needed", default=None)
    web_scrap: Optional[List[str]] = Field(description="List of URLs to scrape if web_scrape tool is needed", default=None)

class SearchIndicator:
    def __init__(self):
        """Initialize the SearchIndicator tool."""
        self.gemini_client = genai.Client(
            api_key=GOOGLE_API_KEY
        )
    
    def analyze_query(self, query_text: str) -> dict:
        """
        Analyzes the user query to determine which tool is most appropriate.
        
        Args:
            query_text (str): The user query text to analyze
            
        Returns:
            dict: A dictionary with tool recommendation and associated parameters
        """
        try:
            system_instruction = """You are an expert at tools indicator. You have access to the following tools:

1. web_search
   - When the user's request requires up-to-date or real-time information.  
   - Parameters:  
     - `query` (string) – A concise query describing the information to be retrieved.

2. video
   - When the user provides valid video URLs (e.g., YouTube links) that require video related processing. 
   - Parameters:  
     - `urls` (string[]) – An array of video URLs.

3. web_scrape
   - When the user provides Non-video URLs (e.g., GitHub, Reddit, news articles) that require direct content extraction.  
   - Parameters:  
     - `urls` (string[]) – An array of webpage URLs to scrape."""
            
            model = "gemini-2.0-flash"
            
            contents = [
                types.Content(
                    role="user",
                    parts=[
                        types.Part.from_text(text=query_text),
                    ],
                ),
            ]
            
            generate_content_config = types.GenerateContentConfig(
                response_mime_type="application/json",
                response_schema=types.Schema(
                    type=types.Type.OBJECT,
                    properties={
                        "tool": types.Schema(
                            type=types.Type.ARRAY,
                            items=types.Schema(
                                type=types.Type.STRING,
                            ),
                        ),
                        "web_search": types.Schema(
                            type=types.Type.STRING,
                        ),
                        "videos": types.Schema(
                            type=types.Type.ARRAY,
                            items=types.Schema(
                                type=types.Type.STRING,
                            ),
                        ),
                        "web_scrap": types.Schema(
                            type=types.Type.ARRAY,
                            items=types.Schema(
                                type=types.Type.STRING,
                            ),
                        ),
                    },
                ),
                system_instruction=[
                    types.Part.from_text(text=system_instruction),
                ],
            )
            
            # Initialize an empty string to collect response chunks
            full_response = ""
            
            # Use streaming to get the response
            for chunk in self.gemini_client.models.generate_content_stream(
                model=model,
                contents=contents,
                config=generate_content_config,
            ):
                if chunk.text:
                    full_response += chunk.text
            
            # Parse the result to a dictionary
            try:
                result = json.loads(full_response)
                
                # Normalize the result to ensure it has the correct structure
                normalized_result = self._normalize_result(result, query_text)
                logger.debug(f"Normalized search indicator result: {normalized_result}")
                
                return normalized_result
                
            except json.JSONDecodeError:
                logger.error(f"Failed to parse search indicator response: {full_response}")
                return self._create_default_web_search(query_text)
            
        except Exception as e:
            # Return a default response in case of error
            logger.error(f"Search indicator error: {str(e)}")
            return self._create_default_web_search(query_text)
    
    def _normalize_result(self, result: dict, query_text: str) -> dict:
        """
        Normalize the search indicator result to ensure it has the correct structure.
        
        Args:
            result: The raw result from the search indicator
            query_text: The original query text
            
        Returns:
            A normalized result dictionary
        """
        normalized = {}
        
        # Handle the case where we have a web_search but no tool
        if "web_search" in result and result["web_search"] and "tool" not in result:
            normalized["tool"] = ["web_search"]
            normalized["web_search"] = result["web_search"]
        
        # Handle case where we have videos but no tool
        elif "videos" in result and result["videos"] and "tool" not in result:
            normalized["tool"] = ["video"]
            normalized["videos"] = result["videos"]
        
        # Handle case where we have web_scrap but no tool
        elif "web_scrap" in result and result["web_scrap"] and "tool" not in result:
            normalized["tool"] = ["web_scrap"]
            normalized["web_scrap"] = result["web_scrap"]
            
        # If tool exists but doesn't match the other keys, fix it
        elif "tool" in result and result["tool"]:
            normalized["tool"] = result["tool"]
            
            # Make sure specific keys are present if tool indicates they should be
            if "web_search" in result["tool"] and "web_search" not in result:
                normalized["web_search"] = query_text
            elif "web_search" in result:
                normalized["web_search"] = result["web_search"]
                
            if "video" in result["tool"]:
                normalized["videos"] = result.get("videos", [])
                
            if "web_scrap" in result["tool"]:
                normalized["web_scrap"] = result.get("web_scrap", [])
        
        # If we have a web_search value but tool doesn't include web_search
        elif "web_search" in result and result["web_search"]:
            if "tool" in result:
                normalized["tool"] = list(set(result["tool"] + ["web_search"]))
            else:
                normalized["tool"] = ["web_search"]
            normalized["web_search"] = result["web_search"]
            
        # If nothing specific matched, copy the original result
        else:
            normalized = result.copy()
            
            # Ensure the tool field exists
            if "tool" not in normalized:
                normalized["tool"] = []
        
        # Copy any other fields we haven't handled
        for key in result:
            if key not in normalized:
                normalized[key] = result[key]
                
        return normalized
    
    def _create_default_web_search(self, query_text: str) -> dict:
        """
        Create a default web search response when error occurs.
        
        Args:
            query_text: The original query text
            
        Returns:
            A default web search dictionary
        """
        return {
            "tool": ["web_search"],
            "web_search": query_text,
            "error": "Created default web search due to error"
        }

# Create a single instance to be used by external callers
search_indicator = SearchIndicator()

def detect_search_needs(query_text: str) -> dict:
    """
    Interface function for detecting if a query needs external tools.
    
    Args:
        query_text (str): The user query text to analyze
        
    Returns:
        dict: Result with recommendation for which tool to use and associated parameters
    """
    if not query_text or len(query_text.strip()) < 3:
        return {"tool": []}
        
    return search_indicator.analyze_query(query_text) 