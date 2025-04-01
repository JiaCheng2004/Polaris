"""
YouTube transcript extraction tool.
"""

from typing import Optional, List, Dict, Union, Any
from youtube_transcript_api import YouTubeTranscriptApi
from langchain.tools import BaseTool
import json

class YouTubeTranscriptTool(BaseTool):
    """Tool for extracting and processing YouTube video transcripts."""
    
    name: str = "youtube_transcript"
    description: str = "Useful for extracting transcripts from YouTube videos. Input should be a YouTube video ID or URL."
    
    def __init__(self):
        """Initialize the YouTube transcript tool."""
        super().__init__()
    
    def _extract_video_id(self, url_or_id: str) -> str:
        """Extract video ID from URL or return if already an ID.
        
        Args:
            url_or_id: YouTube URL or video ID
            
        Returns:
            Video ID string
        """
        if 'youtube.com' in url_or_id or 'youtu.be' in url_or_id:
            # Handle different YouTube URL formats
            if 'youtube.com/watch?v=' in url_or_id:
                return url_or_id.split('watch?v=')[1].split('&')[0]
            elif 'youtu.be/' in url_or_id:
                return url_or_id.split('youtu.be/')[1].split('?')[0]
        return url_or_id
    
    def _run(self, video_url_or_id: str, languages: Optional[List[str]] = None) -> Dict[str, Any]:
        """Run the transcript extraction.
        
        Args:
            video_url_or_id: YouTube video URL or ID
            languages: Optional list of language codes in priority order
            
        Returns:
            Dictionary containing transcript data
        """
        try:
            video_id = self._extract_video_id(video_url_or_id)
            
            # Get transcript - use the static method directly
            transcript = YouTubeTranscriptApi.get_transcript(
                video_id,
                languages=languages or ['en']
            )
            
            # Format transcript directly with json.dumps
            formatted_transcript = json.dumps(transcript)
            
            return {
                'success': True,
                'video_id': video_id,
                'transcript': formatted_transcript,
                'raw_transcript': transcript
            }
            
        except Exception as e:
            return {
                'success': False,
                'error': str(e),
                'video_id': self._extract_video_id(video_url_or_id)
            }
    
    async def _arun(self, video_url_or_id: str, languages: Optional[List[str]] = None) -> Dict[str, Any]:
        """Run the transcript extraction asynchronously.
        
        Args:
            video_url_or_id: YouTube video URL or ID
            languages: Optional list of language codes in priority order
            
        Returns:
            Dictionary containing transcript data
        """
        # Since YouTubeTranscriptApi doesn't have async methods, we'll just call the sync version
        return self._run(video_url_or_id, languages) 