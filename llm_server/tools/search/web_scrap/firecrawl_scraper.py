"""
Firecrawl web scraping tool implementation using LangChain's FireCrawlLoader.
"""

from typing import Optional, Dict, Any, List
import traceback
from langchain_community.document_loaders.firecrawl import FireCrawlLoader
from langchain.tools import BaseTool
from tools.config.load import FIRECRAWL_API_KEY
from tools.logger import logger
from pydantic import Field

class FirecrawlScraperTool(BaseTool):
    """Tool for scraping web content using Firecrawl via LangChain integration."""
    
    name: str = "firecrawl_scraper"
    description: str = "Useful for scraping content from websites. Input should be a URL."
    api_key: Optional[str] = Field(default=None, description="API key for Firecrawl")
    
    def __init__(self, api_key: Optional[str] = None, **kwargs):
        """Initialize the Firecrawl scraper tool.
        
        Args:
            api_key: Optional Firecrawl API key. If not provided, will use from config.
        """
        # Set API key before calling super().__init__()
        _api_key = api_key or FIRECRAWL_API_KEY
        
        # Log API key presence but not the actual key
        if _api_key:
            logger.info("Firecrawl API key is configured")
        else:
            logger.warning("Firecrawl API key is not configured")
            
        # Initialize the BaseTool with our fields
        super().__init__(api_key=_api_key, **kwargs)
    
    def _run(self, url: str) -> Dict[str, Any]:
        """Run the web scraping process using LangChain's FireCrawlLoader.
        
        Args:
            url: The URL to scrape
            
        Returns:
            Dictionary containing scraped content and metadata
        """
        logger.info(f"Starting Firecrawl web scraping for URL: {url}")
        
        if not self.api_key:
            logger.error("Cannot perform web scraping: Firecrawl API key is missing")
            return {
                'success': False,
                'error': "API key is missing",
                'url': url
            }
            
        try:
            # Initialize the FireCrawlLoader
            loader = FireCrawlLoader(
                api_key=self.api_key, 
                url=url, 
                mode="scrape"  # Use "scrape" mode for single-page content
            )
            
            # Load documents
            logger.debug(f"Loading documents from URL: {url}")
            documents = loader.load()
            
            # Log the number of documents loaded
            logger.info(f"Successfully loaded {len(documents)} documents from URL: {url}")
            
            # Prepare the response data
            data_items = []
            
            # Process each document
            for doc in documents:
                # Extract the content and metadata
                markdown_content = doc.page_content
                metadata = doc.metadata
                
                # Add to data items
                data_items.append({
                    'markdown': markdown_content,
                    'html': '',  # FireCrawlLoader doesn't return HTML directly
                    'metadata': metadata
                })
                
                # Log document metadata keys for debugging
                logger.debug(f"Document metadata keys: {', '.join(metadata.keys())}")
            
            # If we have data, return it
            if data_items:
                return {
                    'success': True,
                    'url': url,
                    'data': data_items
                }
            else:
                # No data was found
                logger.warning(f"No content extracted from URL: {url}")
                return {
                    'success': False,
                    'error': 'No content extracted',
                    'url': url
                }
                
        except Exception as e:
            logger.error(f"Error during web scraping: {str(e)}")
            logger.error(f"Traceback: {traceback.format_exc()}")
            return {
                'success': False,
                'error': str(e),
                'url': url
            }
    
    async def _arun(self, url: str) -> Dict[str, Any]:
        """Run the web scraping process asynchronously.
        
        Args:
            url: The URL to scrape
            
        Returns:
            Dictionary containing scraped content and metadata
        """
        # Since FireCrawlLoader doesn't have async methods, we'll just call the sync version
        logger.info(f"Running sync Firecrawl scraper as fallback for async request: {url}")
        return self._run(url) 