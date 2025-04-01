"""
Linkup search implementation using LangChain.
"""

from typing import Optional, Dict, Any
import traceback
from langchain_linkup import LinkupSearchTool as LangchainLinkupTool
from tools.config.load import LINKUP_API_KEY
from tools.logger import logger

class LinkupSearchTool:
    """Tool for performing web searches using Linkup API."""
    
    def __init__(
        self,
        depth: str = "deep",
        output_type: str = "searchResults",
        api_key: Optional[str] = None
    ):
        """Initialize the Linkup search tool.
        
        Args:
            depth: Search depth, either "standard" or "deep"
            output_type: Output type, either "searchResults", "sourcedAnswer" or "structured"
            api_key: Optional Linkup API key. If not provided, will use from config.
        """
        self.api_key = api_key or LINKUP_API_KEY 
        self.depth = depth
        self.output_type = output_type
        
        # Log API key presence but not the actual key
        if self.api_key:
            logger.info("Linkup API key is configured")
        else:
            logger.warning("Linkup API key is not configured")
            
        try:
            self.tool = LangchainLinkupTool(
                depth=depth,
                output_type=output_type,
                linkup_api_key=self.api_key
            )
            logger.info(f"LinkupSearchTool initialized successfully with depth={depth}, output_type={output_type}")
        except Exception as e:
            logger.error(f"Error initializing LinkupSearchTool: {str(e)}")
            logger.error(f"Traceback: {traceback.format_exc()}")
            self.tool = None
    
    def _run(self, query: str) -> Dict[str, Any]:
        """Run the search query.
        
        Args:
            query: The search query string
            
        Returns:
            Dictionary containing search results
        """
        logger.info(f"Starting Linkup search for query: {query}")
        
        if not self.tool:
            logger.error("Linkup search tool was not properly initialized")
            return {"results": [], "error": "Search tool not initialized"}
            
        if not self.api_key:
            logger.error("Cannot perform search: Linkup API key is missing")
            return {"results": [], "error": "API key is missing"}
            
        try:
            # Invoke the search tool
            logger.debug(f"Invoking Linkup search tool with query: {query}")
            raw_results = self.tool.invoke({"query": query})
            logger.debug(f"Linkup search returned result type: {type(raw_results).__name__}")
            
            # Process the results based on the output type
            if self.output_type == "searchResults":
                # Format the results for consistent handling
                if hasattr(raw_results, "results") and raw_results.results:
                    results = raw_results.results
                    logger.info(f"Linkup search returned {len(results)} results")
                    
                    # Standardize format for processing
                    standardized_results = []
                    for result in results:
                        if hasattr(result, "name") and hasattr(result, "url") and hasattr(result, "content"):
                            standardized_results.append({
                                "title": result.name,
                                "url": result.url,
                                "content": result.content
                            })
                        else:
                            # Handle unexpected result format
                            logger.warning(f"Unexpected Linkup result format: {type(result).__name__}")
                            standardized_results.append({
                                "title": "Search Result",
                                "url": getattr(result, "url", "https://linkup.com"),
                                "content": str(result)
                            })
                    
                    return {
                        "results": standardized_results,
                        "query": query,
                        "success": True
                    }
                else:
                    logger.warning("Linkup search returned no results")
                    return {
                        "results": [],
                        "query": query,
                        "success": True
                    }
            else:
                # For other output types, use raw results
                logger.info(f"Using raw Linkup results for output_type={self.output_type}")
                return {
                    "results": [{
                        "title": "Linkup Search",
                        "url": "https://linkup.com",
                        "content": str(raw_results)
                    }],
                    "query": query,
                    "success": True
                }
        except Exception as e:
            logger.error(f"Error during Linkup search: {str(e)}")
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
            Dictionary containing search results
        """
        # Since LangchainLinkupTool doesn't have async methods, we'll just call the sync version
        logger.info(f"Running sync Linkup search as fallback for async request: {query}")
        return self._run(query) 