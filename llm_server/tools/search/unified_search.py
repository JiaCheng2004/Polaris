"""
Unified search interface that coordinates different search tools.
"""

import json
from typing import Optional, List, Dict, Any

# Import search indicator
from tools.llm.search_indicator import detect_search_needs

# Import search configuration
from tools.config.load import SEARCH_PREFERENCE, TAVILY_API_KEY, LINKUP_API_KEY, FIRECRAWL_API_KEY

# Import search tools
from .web_search import TavilySearchTool, LinkupSearchTool
from .video import YouTubeTranscriptTool
from .web_scrap import FirecrawlScraperTool

def get_preferred_search_tool():
    """Get the preferred search tool from config."""
    from tools.logger import logger
    
    logger.info(f"Getting preferred search tool, preference is: {SEARCH_PREFERENCE}")
    
    try:
        if SEARCH_PREFERENCE == "linkup" and LINKUP_API_KEY:
            logger.info("Using LinkupSearchTool for web search")
            return LinkupSearchTool(api_key=LINKUP_API_KEY)
        elif SEARCH_PREFERENCE == "tavily" and TAVILY_API_KEY:
            logger.info("Using TavilySearchTool for web search")
            return TavilySearchTool(api_key=TAVILY_API_KEY)
        else:
            # Try to pick one that has an API key configured
            if TAVILY_API_KEY:
                logger.info("Using TavilySearchTool as fallback (has API key)")
                return TavilySearchTool(api_key=TAVILY_API_KEY)
            elif LINKUP_API_KEY:
                logger.info("Using LinkupSearchTool as fallback (has API key)")
                return LinkupSearchTool(api_key=LINKUP_API_KEY)
            else:
                logger.warning("No API keys configured for any search tool")
                # Return a tool that will handle the missing API key gracefully
                return TavilySearchTool(api_key="")
    except Exception as e:
        logger.error(f"Error getting preferred search tool: {str(e)}")
        # Return a tool that will handle errors gracefully
        return TavilySearchTool(api_key="")

def _format_web_search_results(results: Dict[str, Any]) -> str:
    """Format web search results into a readable text format."""
    from tools.logger import logger
    
    formatted_text = "### WEB SEARCH RESULTS\n\n"
    
    # Debug log for seeing raw search results
    logger.debug(f"Raw web search results: {results}")
    
    if not results:
        logger.warning("Web search returned empty results")
        return formatted_text + "No results found.\n\n"
    
    if not results.get("results"):
        logger.warning(f"Web search results missing 'results' key: {results.keys()}")
        return formatted_text + "No results found.\n\n"
    
    if len(results.get("results", [])) == 0:
        logger.warning("Web search returned empty results array")
        return formatted_text + "No results found.\n\n"
    
    try:
        # Log the number and type of results
        results_count = len(results.get("results", []))
        first_result_type = type(results.get("results", [])[0]).__name__ if results.get("results") else "None"
        logger.debug(f"Processing {results_count} web search results of type {first_result_type}")
        
        for i, result in enumerate(results.get("results", []), 1):
            if hasattr(result, "name") and hasattr(result, "url") and hasattr(result, "content"):
                # Linkup format
                logger.debug(f"Processing Linkup result {i}: {result.name[:30]}...")
                formatted_text += f"{i}. **{result.name}**\n"
                formatted_text += f"   URL: {result.url}\n"
                formatted_text += f"   {result.content}\n\n"
            elif isinstance(result, dict):
                # Tavily format
                result_title = result.get('title', 'No Title')
                logger.debug(f"Processing Tavily result {i}: {result_title[:30]}...")
                formatted_text += f"{i}. **{result_title}**\n"
                formatted_text += f"   URL: {result.get('url', 'No URL')}\n"
                formatted_text += f"   {result.get('content', 'No content')}\n\n"
            else:
                # Unknown format
                logger.warning(f"Unknown result format at index {i}: {type(result).__name__}")
                formatted_text += f"{i}. **Unknown Format**\n"
                formatted_text += f"   {str(result)[:500]}\n\n"
    
        # Log the final formatted text length
        logger.debug(f"Formatted web search results: {len(formatted_text)} chars")
        if len(formatted_text) < 200:
            logger.warning(f"Suspiciously short search results: '{formatted_text}'")
    except Exception as e:
        logger.error(f"Error formatting web search results: {str(e)}")
        return formatted_text + f"Error formatting results: {str(e)}\n\n"
    
    return formatted_text

def _format_video_results(results: Dict[str, Any]) -> str:
    """Format video transcript results into a readable text format."""
    formatted_text = "### VIDEO TRANSCRIPT\n\n"
    
    if not results or not results.get("success", False):
        return formatted_text + "Could not retrieve video transcript.\n\n"
    
    video_id = results.get("video_id", "Unknown")
    formatted_text += f"Video ID: {video_id}\n\n"
    
    if results.get("transcript"):
        try:
            # Try to parse JSON if it's a string
            if isinstance(results["transcript"], str):
                transcript_data = json.loads(results["transcript"])
                if isinstance(transcript_data, list):
                    # If it's a list of transcript segments
                    transcript_text = ""
                    for segment in transcript_data:
                        transcript_text += segment.get("text", "") + " "
                    formatted_text += transcript_text
                else:
                    formatted_text += str(transcript_data)
            else:
                formatted_text += str(results["transcript"])
        except Exception:
            # If parsing fails, just use the raw transcript
            formatted_text += str(results["transcript"])
    else:
        formatted_text += "No transcript available."
    
    return formatted_text + "\n\n"

def _format_web_scrape_results(results: Dict[str, Any]) -> str:
    """Format web scraping results into a readable text format."""
    formatted_text = "### WEB CONTENT\n\n"
    
    if not results or not results.get("success", False):
        return formatted_text + "Could not retrieve web content.\n\n"
    
    url = results.get("url", "Unknown URL")
    formatted_text += f"Source: {url}\n\n"
    
    if results.get("data"):
        for item in results["data"]:
            # Prefer markdown format if available
            content = item.get("markdown", item.get("html", "No content available"))
            # Truncate if too long
            if len(content) > 10000:
                content = content[:10000] + "... [content truncated]"
            formatted_text += content + "\n\n"
    else:
        formatted_text += "No content available."
    
    return formatted_text

def unified_search(query: str) -> str:
    """
    Unified search interface that determines the appropriate tools to use
    and returns combined results as a single text block.
    
    Args:
        query: The user query
        
    Returns:
        A formatted string containing search results
    """
    try:
        # Use search indicator to determine which tools to use
        search_results = detect_search_needs(query)
        
        if not search_results:
            return "No search tools were determined to be relevant for this query."
        
        combined_results = f"Search results for: {query}\n\n"
        performed_search = False
        
        # Process web search if needed - check for web_search key directly
        if search_results.get("web_search"):
            web_search_query = search_results.get("web_search", query)
            web_search_tool = get_preferred_search_tool()
            web_results = web_search_tool._run(web_search_query)
            combined_results += _format_web_search_results(web_results)
            performed_search = True
        
        # Get tools list if available
        tools_to_use = search_results.get("tool", [])
        
        # Process web search via tools list
        if "web_search" in tools_to_use and not performed_search:
            web_search_query = search_results.get("web_search", query)
            web_search_tool = get_preferred_search_tool()
            web_results = web_search_tool._run(web_search_query)
            combined_results += _format_web_search_results(web_results)
            performed_search = True
        
        # Process video transcripts if needed
        if "video" in tools_to_use or search_results.get("videos"):
            video_tool = YouTubeTranscriptTool()
            video_urls = search_results.get("videos", [])
            
            if video_urls:
                for video_url in video_urls:
                    video_results = video_tool._run(video_url)
                    combined_results += _format_video_results(video_results)
                performed_search = True
        
        # Process web scraping if needed
        if "web_scrap" in tools_to_use or search_results.get("web_scrap"):
            web_scrape_tool = FirecrawlScraperTool(api_key=FIRECRAWL_API_KEY)
            urls = search_results.get("web_scrap", [])
            
            if urls:
                for url in urls:
                    scrape_results = web_scrape_tool._run(url)
                    combined_results += _format_web_scrape_results(scrape_results)
                performed_search = True
        
        # If no search was performed, search the web using the original query as fallback
        if not performed_search and query:
            web_search_tool = get_preferred_search_tool()
            web_results = web_search_tool._run(query)
            combined_results += _format_web_search_results(web_results)
        
        return combined_results
    
    except Exception as e:
        # Return error message if something goes wrong
        return f"An error occurred during search: {str(e)}" 