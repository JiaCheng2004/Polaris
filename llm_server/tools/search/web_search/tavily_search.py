"""
Tavily search implementation using LangChain.
"""

from typing import Optional, List, Dict, Any
import traceback
from langchain.tools import BaseTool
from langchain_community.tools.tavily_search.tool import TavilySearchResults
from tools.config.load import TAVILY_API_KEY
from tools.logger import logger
from pydantic import Field

class TavilySearchTool(BaseTool):
    """Tool for performing web searches using Tavily API."""
    
    name: str = "tavily_search"
    description: str = "Useful for searching the web for current information. Input should be a search query."
    api_key: Optional[str] = Field(default=None, description="API key for Tavily")
    max_results: int = Field(default=5, description="Maximum number of results to return")
    search_tool: Optional[Any] = Field(default=None, exclude=True)
    
    def __init__(self, api_key: Optional[str] = None, max_results: int = 5, **kwargs):
        """Initialize the Tavily search tool.
        
        Args:
            api_key: Optional Tavily API key. If not provided, will use from config.
            max_results: Maximum number of results to return (default: 5)
        """
        # Set API key before calling super().__init__()
        _api_key = api_key or TAVILY_API_KEY
        
        # Log API key presence but not the actual key
        if _api_key:
            logger.info("Tavily API key is configured")
        else:
            logger.warning("Tavily API key is not configured")
        
        # Initialize the BaseTool with our fields
        super().__init__(api_key=_api_key, max_results=max_results, **kwargs)
        
        # Initialize the search tool
        self._initialize_search_tool()
    
    def _initialize_search_tool(self):
        """Initialize the underlying Tavily search tool."""
        try:
            self.search_tool = TavilySearchResults(
                api_key=self.api_key,
                max_results=self.max_results,
                include_raw_content=True,
                search_depth="advanced"
            )
            logger.info("TavilySearchResults tool initialized successfully")
        except Exception as e:
            logger.error(f"Error initializing TavilySearchResults tool: {str(e)}")
            logger.error(f"Traceback: {traceback.format_exc()}")
            self.search_tool = None
    
    def _run(self, query: str) -> Dict[str, Any]:
        """Run the search query.
        
        Args:
            query: The search query string
            
        Returns:
            Dictionary with search results
        """
        logger.info(f"Starting Tavily search for query: {query}")
        
        if not self.search_tool:
            logger.error("Tavily search tool was not properly initialized")
            return {"results": [], "error": "Search tool not initialized"}
            
        if not self.api_key:
            logger.error("Cannot perform search: Tavily API key is missing")
            return {"results": [], "error": "API key is missing"}
            
        try:
            # Direct call to invoke
            logger.debug(f"Invoking Tavily search tool with query: {query}")
            raw_results = self.search_tool.invoke(query)
            logger.debug(f"Tavily search returned result type: {type(raw_results).__name__}")
            
            # Ensure results are properly formatted
            if isinstance(raw_results, list):
                # Results already in the expected format
                results = raw_results
                logger.info(f"Tavily search returned {len(results)} results")
            elif isinstance(raw_results, dict) and "results" in raw_results:
                # Results wrapped in a dict with "results" key
                results = raw_results["results"]
                logger.info(f"Tavily search returned {len(results)} results (wrapped in dict)")
            else:
                # Unexpected format, try to recover
                logger.warning(f"Unexpected Tavily result format: {type(raw_results).__name__}")
                if hasattr(raw_results, "to_dict"):
                    logger.debug("Attempting to convert results using to_dict()")
                    results = raw_results.to_dict()
                else:
                    # Last resort: convert to string and wrap in a fake result
                    logger.warning("Using string representation as fallback")
                    results = [{
                        "title": "Search Results",
                        "url": "https://tavily.com",
                        "content": str(raw_results)
                    }]
            
            # Return standardized format
            return {
                "results": results,
                "query": query,
                "success": True
            }
        except Exception as e:
            logger.error(f"Error during Tavily search: {str(e)}")
            logger.error(f"Traceback: {traceback.format_exc()}")
            return {
                "results": [],
                "error": str(e),
                "query": query,
                "success": False
            }
    
    async def _arun(self, query: str) -> Dict[str, Any]:
        """Run the search query asynchronously.
        
        Args:
            query: The search query string
            
        Returns:
            Dictionary with search results
        """
        logger.info(f"Starting async Tavily search for query: {query}")
        
        if not self.search_tool:
            logger.error("Tavily search tool was not properly initialized")
            return {"results": [], "error": "Search tool not initialized"}
            
        try:
            # Attempt async call
            logger.debug(f"Invoking async Tavily search tool with query: {query}")
            raw_results = await self.search_tool.ainvoke(query)
            
            # Process results same as sync version
            if isinstance(raw_results, list):
                results = raw_results
            elif isinstance(raw_results, dict) and "results" in raw_results:
                results = raw_results["results"]
            else:
                if hasattr(raw_results, "to_dict"):
                    results = raw_results.to_dict()
                else:
                    results = [{
                        "title": "Search Results",
                        "url": "https://tavily.com",
                        "content": str(raw_results)
                    }]
            
            return {
                "results": results,
                "query": query,
                "success": True
            }
        except Exception as e:
            logger.error(f"Error during async Tavily search: {str(e)}")
            return {
                "results": [],
                "error": str(e),
                "query": query,
                "success": False
            } 