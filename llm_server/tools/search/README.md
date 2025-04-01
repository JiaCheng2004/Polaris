# Search Tools

This module provides a unified interface for various search tools including web search, video transcription, and web scraping.

## Overview

The search tools module offers a single entry point (`unified_search`) that automatically determines which search tools to use based on the query, retrieves information from the appropriate sources, and returns a unified text response.

## Components

### Unified Search Interface

The `unified_search` function acts as the main entry point. It:

1. Analyzes the user query using the search indicator tool
2. Determines which specialized tools to use (web search, video, web scraping)
3. Executes the appropriate tools
4. Formats and combines the results into a single text response

### Web Search Tools

- **TavilySearchTool**: Web search using Tavily API
- **LinkupSearchTool**: Web search using Linkup API

The system will use the preferred search tool as specified in the configuration (defaults to Tavily).

### Video Tools

- **YouTubeTranscriptTool**: Extracts and processes transcripts from YouTube videos

### Web Scraping Tools

- **FirecrawlScraperTool**: Scrapes and extracts content from web pages using Firecrawl API

## Usage

```python
from tools.search import unified_search

# Simple text query
results = unified_search("What are the latest developments in quantum computing?")

# Query with video
results = unified_search("Explain the video https://www.youtube.com/watch?v=dQw4w9WgXcQ")

# Query with specific website
results = unified_search("Summarize the information on https://example.com/article")
```

## Configuration

The search tools use configuration settings loaded from `config.json`:

- `search_preference`: Preferred web search tool ("tavily" or "linkup")

API keys are loaded from environment variables:

- `TAVILY_API_KEY`
- `LINKUP_API_KEY`
- `FIRECRAWL_API_KEY`

## Search Indicator

The system uses a search indicator tool powered by Google's Gemini model to analyze queries and determine which tools to use. This ensures optimal tool selection for each query type. 